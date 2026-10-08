"""Upload storage shared with the Go API (internal/storage).

Locators persisted in the database are either absolute local paths (the
historical /tmp/selar_uploads layout used by Docker Compose) or
``s3://<bucket>/<key>`` for S3-compatible object storage (Cloudflare R2,
Supabase Storage, AWS S3, MinIO).

Keys match the API: ``<document_id>.pdf`` for PDFs uploaded through the API,
``users/<user_id>/uploads/<upload_id>.pdf`` for direct browser uploads and
``<document_id>/assets/<sha256>.<ext>`` for extracted visual assets.

boto3 is imported lazily so local mode and the test suite never need it.
Secrets are read from the environment and never logged or put in errors.
"""

from __future__ import annotations

import contextlib
import os
import shutil
import tempfile
from pathlib import Path
from typing import Any, Callable, Iterator, Mapping, Optional, Protocol

DEFAULT_LOCAL_ROOT = "/tmp/selar_uploads"


class StorageError(Exception):
    """A storage operation failed. Messages never include credentials."""


class InvalidLocator(StorageError):
    pass


def asset_prefix(document_id: str) -> str:
    return f"{document_id}/assets/"


class Storage(Protocol):
    backend: str

    def put_bytes(self, key: str, data: bytes, content_type: str) -> str: ...
    def read_bytes(self, locator: str) -> bytes: ...
    def exists(self, locator: str) -> bool: ...
    def delete_prefix(self, prefix: str) -> None: ...
    def local_copy(self, locator: str) -> contextlib.AbstractContextManager[str]: ...


class LocalStorage:
    backend = "local"

    def __init__(self, root: str = DEFAULT_LOCAL_ROOT):
        self.root = Path(root)

    def _path_for_key(self, key: str) -> Path:
        candidate = (self.root / key).resolve()
        root = self.root.resolve()
        if candidate != root and root not in candidate.parents:
            raise InvalidLocator("storage key escapes the upload root")
        return candidate

    def _resolve(self, locator: str) -> Path:
        if not os.path.isabs(locator):
            raise InvalidLocator("local storage locators must be absolute paths")
        candidate = Path(locator).resolve()
        root = self.root.resolve()
        if candidate != root and root not in candidate.parents:
            raise InvalidLocator("locator is outside the upload root")
        return candidate

    def put_bytes(self, key: str, data: bytes, content_type: str) -> str:
        path = self._path_for_key(key)
        path.parent.mkdir(parents=True, exist_ok=True)
        if not path.exists():
            path.write_bytes(data)
        # Keep the historical (unresolved) root form so stored paths stay
        # stable across symlinked temp directories.
        return str(self.root / key)

    def read_bytes(self, locator: str) -> bytes:
        return self._resolve(locator).read_bytes()

    def exists(self, locator: str) -> bool:
        try:
            return self._resolve(locator).exists()
        except InvalidLocator:
            return False

    def delete_prefix(self, prefix: str) -> None:
        target = self._path_for_key(prefix.rstrip("/"))
        shutil.rmtree(target, ignore_errors=True)
        # Assets used to live in <root>/<doc>/assets; removing <doc> keeps the
        # pre-abstraction cleanup behaviour.
        parent = target.parent
        if parent != self.root.resolve() and prefix.endswith("/assets/"):
            shutil.rmtree(parent, ignore_errors=True)

    @contextlib.contextmanager
    def local_copy(self, locator: str) -> Iterator[str]:
        path = self._resolve(locator)
        if not path.exists():
            raise FileNotFoundError(f"file {locator} not found")
        yield str(path)


class S3Storage:
    backend = "s3"

    def __init__(self, bucket: str, client: Any):
        self.bucket = bucket
        self.client = client

    @classmethod
    def from_config(cls, *, endpoint: str, bucket: str, region: str, access_key_id: str,
                    secret_access_key: str, force_path_style: bool = False) -> "S3Storage":
        try:
            import boto3  # noqa: PLC0415 - optional dependency
            from botocore.config import Config  # noqa: PLC0415
        except ImportError as exc:  # pragma: no cover - exercised only without boto3
            raise StorageError("STORAGE_BACKEND=s3 requires the boto3 package") from exc
        client = boto3.client(
            "s3",
            endpoint_url=endpoint if "://" in endpoint else f"https://{endpoint}",
            region_name=region or "auto",
            aws_access_key_id=access_key_id,
            aws_secret_access_key=secret_access_key,
            config=Config(
                signature_version="s3v4",
                s3={"addressing_style": "path" if force_path_style else "auto"},
                retries={"max_attempts": 3, "mode": "standard"},
            ),
        )
        return cls(bucket, client)

    def locator(self, key: str) -> str:
        return f"s3://{self.bucket}/{key}"

    def _key(self, locator: str) -> str:
        prefix = f"s3://{self.bucket}/"
        if not locator.startswith(prefix):
            raise InvalidLocator("locator does not belong to the configured bucket")
        key = locator[len(prefix):]
        if not key or ".." in key.split("/") or key.startswith("/"):
            raise InvalidLocator("invalid object key")
        return key

    @staticmethod
    def _is_missing(exc: Exception) -> bool:
        response = getattr(exc, "response", None) or {}
        code = str(response.get("Error", {}).get("Code", ""))
        return code in {"404", "NoSuchKey", "NotFound"}

    def put_bytes(self, key: str, data: bytes, content_type: str) -> str:
        self._key(self.locator(key))
        try:
            self.client.put_object(Bucket=self.bucket, Key=key, Body=data, ContentType=content_type)
        except Exception as exc:
            raise StorageError(f"object storage upload failed ({type(exc).__name__})") from None
        return self.locator(key)

    def read_bytes(self, locator: str) -> bytes:
        key = self._key(locator)
        try:
            response = self.client.get_object(Bucket=self.bucket, Key=key)
        except Exception as exc:
            if self._is_missing(exc):
                raise FileNotFoundError(f"file {locator} not found") from None
            raise StorageError(f"object storage read failed ({type(exc).__name__})") from None
        return response["Body"].read()

    def exists(self, locator: str) -> bool:
        try:
            key = self._key(locator)
        except InvalidLocator:
            return False
        try:
            self.client.head_object(Bucket=self.bucket, Key=key)
            return True
        except Exception as exc:
            if self._is_missing(exc):
                return False
            raise StorageError(f"object storage lookup failed ({type(exc).__name__})") from None

    def delete_prefix(self, prefix: str) -> None:
        token: Optional[str] = None
        while True:
            kwargs: dict[str, Any] = {"Bucket": self.bucket, "Prefix": prefix}
            if token:
                kwargs["ContinuationToken"] = token
            page = self.client.list_objects_v2(**kwargs)
            for item in page.get("Contents", []) or []:
                self.client.delete_object(Bucket=self.bucket, Key=item["Key"])
            if not page.get("IsTruncated"):
                return
            token = page.get("NextContinuationToken")

    @contextlib.contextmanager
    def local_copy(self, locator: str) -> Iterator[str]:
        """Download to a private temp file (PDF parsers need a real path)."""
        data = self.read_bytes(locator)
        handle, path = tempfile.mkstemp(prefix="selar-", suffix=".pdf")
        try:
            with os.fdopen(handle, "wb") as file:
                file.write(data)
            yield path
        finally:
            with contextlib.suppress(FileNotFoundError):
                os.remove(path)


def from_env(env: Mapping[str, str] | None = None,
             s3_factory: Callable[..., S3Storage] = S3Storage.from_config) -> Storage:
    env = os.environ if env is None else env
    backend = (env.get("STORAGE_BACKEND") or "local").strip().lower()
    if backend == "local":
        return LocalStorage((env.get("LOCAL_STORAGE_DIR") or DEFAULT_LOCAL_ROOT).strip())
    if backend != "s3":
        raise StorageError("STORAGE_BACKEND must be local or s3")
    values = {
        name: (env.get(name) or "").strip()
        for name in ("S3_ENDPOINT", "S3_BUCKET", "S3_REGION", "S3_ACCESS_KEY_ID", "S3_SECRET_ACCESS_KEY")
    }
    missing = sorted(name for name in ("S3_ENDPOINT", "S3_BUCKET", "S3_ACCESS_KEY_ID", "S3_SECRET_ACCESS_KEY")
                     if not values[name])
    if missing:
        raise StorageError(f"STORAGE_BACKEND=s3 requires {', '.join(missing)}")
    return s3_factory(
        endpoint=values["S3_ENDPOINT"], bucket=values["S3_BUCKET"], region=values["S3_REGION"] or "auto",
        access_key_id=values["S3_ACCESS_KEY_ID"], secret_access_key=values["S3_SECRET_ACCESS_KEY"],
        force_path_style=(env.get("S3_FORCE_PATH_STYLE") or "").strip().lower() == "true",
    )


_configured: Optional[Storage] = None


def get_storage() -> Storage:
    global _configured
    if _configured is None:
        _configured = from_env()
    return _configured


def set_storage(storage: Optional[Storage]) -> None:
    """Override the process-wide storage (tests, Modal app wiring)."""
    global _configured
    _configured = storage

"""Storage abstraction tests. No network: S3 uses an in-memory fake client."""

from pathlib import Path

import pytest

import storage
from storage import InvalidLocator, LocalStorage, S3Storage, StorageError, from_env


class _Missing(Exception):
    def __init__(self):
        super().__init__("missing")
        self.response = {"Error": {"Code": "NoSuchKey"}}


class _Body:
    def __init__(self, data: bytes):
        self._data = data

    def read(self) -> bytes:
        return self._data


class FakeS3Client:
    def __init__(self):
        self.objects: dict[str, tuple[bytes, str]] = {}

    def put_object(self, *, Bucket, Key, Body, ContentType):
        assert Bucket == "selar-test"
        self.objects[Key] = (Body, ContentType)

    def get_object(self, *, Bucket, Key):
        if Key not in self.objects:
            raise _Missing()
        return {"Body": _Body(self.objects[Key][0])}

    def head_object(self, *, Bucket, Key):
        if Key not in self.objects:
            raise _Missing()
        return {"ContentLength": len(self.objects[Key][0])}

    def list_objects_v2(self, *, Bucket, Prefix, ContinuationToken=None):
        keys = sorted(key for key in self.objects if key.startswith(Prefix))
        return {"Contents": [{"Key": key} for key in keys], "IsTruncated": False}

    def delete_object(self, *, Bucket, Key):
        self.objects.pop(Key, None)


def test_default_backend_is_the_legacy_local_directory():
    store = from_env({})
    assert isinstance(store, LocalStorage)
    assert str(store.root) == "/tmp/selar_uploads"


def test_s3_config_errors_name_variables_but_never_values():
    with pytest.raises(StorageError) as error:
        from_env({"STORAGE_BACKEND": "s3", "S3_BUCKET": "b", "S3_SECRET_ACCESS_KEY": "super-secret-value"})
    assert "S3_ENDPOINT" in str(error.value)
    assert "super-secret-value" not in str(error.value)
    with pytest.raises(StorageError):
        from_env({"STORAGE_BACKEND": "ftp"})


def test_s3_config_is_passed_to_factory():
    seen = {}

    def factory(**kwargs):
        seen.update(kwargs)
        return S3Storage(kwargs["bucket"], FakeS3Client())

    store = from_env({
        "STORAGE_BACKEND": "s3", "S3_ENDPOINT": "https://acct.r2.cloudflarestorage.com", "S3_BUCKET": "selar-test",
        "S3_ACCESS_KEY_ID": "id", "S3_SECRET_ACCESS_KEY": "secret", "S3_FORCE_PATH_STYLE": "true",
    }, s3_factory=factory)
    assert store.backend == "s3"
    assert seen["region"] == "auto" and seen["force_path_style"] is True


def test_local_round_trip_and_asset_cleanup(tmp_path: Path):
    store = LocalStorage(str(tmp_path))
    locator = store.put_bytes("doc-1/assets/abc.png", b"png-bytes", "image/png")
    assert locator == str(tmp_path / "doc-1/assets/abc.png")
    assert store.exists(locator) and store.read_bytes(locator) == b"png-bytes"
    store.delete_prefix(storage.asset_prefix("doc-1"))
    assert not (tmp_path / "doc-1").exists()


def test_local_rejects_paths_outside_root(tmp_path: Path):
    store = LocalStorage(str(tmp_path / "root"))
    for locator in ("/etc/passwd", str(tmp_path / "root" / ".." / "escape.pdf"), "relative.pdf", "s3://b/k"):
        with pytest.raises(InvalidLocator):
            store.read_bytes(locator)
    with pytest.raises(InvalidLocator):
        store.put_bytes("../escape.png", b"x", "image/png")


def test_local_copy_of_local_file_is_the_file_itself(tmp_path: Path):
    pdf = tmp_path / "doc.pdf"
    pdf.write_bytes(b"%PDF-1.4")
    with LocalStorage(str(tmp_path)).local_copy(str(pdf)) as path:
        assert path == str(pdf.resolve())
    with pytest.raises(FileNotFoundError):
        with LocalStorage(str(tmp_path)).local_copy(str(tmp_path / "missing.pdf")):
            pass


def test_s3_round_trip_local_copy_and_prefix_delete():
    client = FakeS3Client()
    store = S3Storage("selar-test", client)
    locator = store.put_bytes("doc-2/assets/a.png", b"png", "image/png")
    assert locator == "s3://selar-test/doc-2/assets/a.png"
    assert client.objects["doc-2/assets/a.png"] == (b"png", "image/png")
    assert store.exists(locator) and store.read_bytes(locator) == b"png"

    client.objects["users/u/uploads/x.pdf"] = (b"%PDF-1.7 synthetic", "application/pdf")
    with store.local_copy("s3://selar-test/users/u/uploads/x.pdf") as path:
        temp_path = path
        assert Path(path).read_bytes() == b"%PDF-1.7 synthetic"
    assert not Path(temp_path).exists(), "temporary download must be removed"

    store.delete_prefix("doc-2/assets/")
    assert "doc-2/assets/a.png" not in client.objects
    assert not store.exists(locator)
    with pytest.raises(FileNotFoundError):
        store.read_bytes(locator)


def test_s3_rejects_foreign_bucket_and_traversal():
    store = S3Storage("selar-test", FakeS3Client())
    for locator in ("s3://other/x.pdf", "/tmp/selar_uploads/x.pdf", "s3://selar-test/../x"):
        with pytest.raises(InvalidLocator):
            store.read_bytes(locator)
    assert store.exists("s3://other/x.pdf") is False


def test_s3_errors_hide_provider_details():
    class Boom(FakeS3Client):
        def put_object(self, **_):
            raise RuntimeError("SignatureDoesNotMatch secret=abc123")

    with pytest.raises(StorageError) as error:
        S3Storage("selar-test", Boom()).put_bytes("k.png", b"x", "image/png")
    assert "abc123" not in str(error.value)

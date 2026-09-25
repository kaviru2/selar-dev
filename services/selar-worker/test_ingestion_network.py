"""Network-boundary regressions; all TCP attempts are intercepted locally."""
import asyncio
import socket
import ssl
import subprocess

import httpcore
import httpx
import httpx._transports.default as default_transport
from httpcore._backends.anyio import AnyIOBackend, AnyIOStream
import pytest

from ingestion import UnsafeSourceURL, canonicalize_url, fetch_public_url, robots_allows


async def _local_fixture(monkeypatch, *, redirect=False, robots=False, proxy=False, image=False):
    received = []

    async def serve(reader, writer):
        request = await reader.readuntil(b"\r\n\r\n")
        received.append(request)
        if redirect:
            response = b"HTTP/1.1 302 Found\r\nLocation: http://rebound.invalid/article\r\nContent-Length: 0\r\n\r\n"
        else:
            response = b"HTTP/1.1 200 OK\r\nContent-Type: text/plain\r\nContent-Length: 2\r\n\r\nOK"
        writer.write(response)
        await writer.drain()
        writer.close()
        await writer.wait_closed()

    server = await asyncio.start_server(serve, "127.0.0.1", 0)
    port = server.sockets[0].getsockname()[1]
    original_resolve = socket.getaddrinfo
    original_connect = AnyIOBackend.connect_tcp
    lookups = []
    attempts = []

    def resolve(host, requested_port, *args, **kwargs):
        if host in {"rebound.invalid", "start.invalid"}:
            lookups.append(host)
            # First DNS check is public; a second hostname lookup rebounds locally.
            address = "8.8.8.8" if lookups.count(host) == 1 else "127.0.0.1"
            return [(socket.AF_INET, socket.SOCK_STREAM, 6, "", (address, requested_port))]
        return original_resolve(host, requested_port, *args, **kwargs)

    async def local_only_connect(self, host, requested_port, **kwargs):
        attempts.append((host, requested_port))
        if host == "start.invalid" or (host == "8.8.8.8" and redirect and len(received) == 0):
            # Route the allowed first request to a synthetic local HTTP server.
            return await original_connect(self, "127.0.0.1", port, **kwargs)
        if host == "rebound.invalid":
            assert resolve(host, requested_port)[0][4][0] == "127.0.0.1"
            return await original_connect(self, "127.0.0.1", port, **kwargs)
        raise httpcore.ConnectError("test forbids external or proxy TCP")

    monkeypatch.setattr(socket, "getaddrinfo", resolve)
    monkeypatch.setattr(AnyIOBackend, "connect_tcp", local_only_connect)
    if proxy:
        for name in ("HTTP_PROXY", "HTTPS_PROXY", "ALL_PROXY", "http_proxy", "https_proxy", "all_proxy"):
            monkeypatch.setenv(name, "http://127.0.0.1:9999")
    try:
        try:
            if robots:
                await robots_allows("http://rebound.invalid/article")
            else:
                await fetch_public_url(
                    "http://start.invalid/article" if redirect else "http://rebound.invalid/article",
                    allowed_content_types=("image/",) if image else ("text/html", "text/plain"),
                )
        except (UnsafeSourceURL, httpx.ConnectError):
            pass
        await asyncio.sleep(0)
        return received, attempts, lookups
    finally:
        server.close()
        await server.wait_closed()


def test_rebound_initial_sends_zero_get_bytes_to_loopback(monkeypatch):
    received, attempts, lookups = asyncio.run(_local_fixture(monkeypatch))
    assert lookups == ["rebound.invalid"]
    assert received == []
    assert attempts == [("8.8.8.8", 80)]


def test_rebound_redirect_sends_zero_get_bytes_to_redirect_target(monkeypatch):
    received, attempts, lookups = asyncio.run(_local_fixture(monkeypatch, redirect=True))
    assert lookups == ["start.invalid", "rebound.invalid"]
    assert len(received) == 1
    assert received[0].startswith(b"GET /article HTTP/1.1\r\nHost: start.invalid\r\n")
    assert attempts == [("8.8.8.8", 80), ("8.8.8.8", 80)]


def test_robots_fetch_cannot_send_get_to_rebound_loopback(monkeypatch):
    received, attempts, lookups = asyncio.run(_local_fixture(monkeypatch, robots=True))
    assert lookups == ["rebound.invalid"]
    assert received == []
    assert attempts == [("8.8.8.8", 80)]


def test_image_fetch_cannot_send_get_to_rebound_loopback(monkeypatch):
    received, attempts, lookups = asyncio.run(_local_fixture(monkeypatch, image=True))
    assert lookups == ["rebound.invalid"]
    assert received == []
    assert attempts == [("8.8.8.8", 80)]


def test_proxy_environment_is_not_used_for_ingestion(monkeypatch):
    received, attempts, lookups = asyncio.run(_local_fixture(monkeypatch, proxy=True))
    assert lookups == ["rebound.invalid"]
    assert received == []
    assert attempts == [("8.8.8.8", 80)]


def test_ipv6_literal_keeps_brackets_and_nondefault_port():
    assert canonicalize_url("https://[2606:4700:4700::1111]:8443/path") == (
        "https://[2606:4700:4700::1111]:8443/path"
    )


def test_ipv6_literal_default_port_is_canonical():
    assert canonicalize_url("https://[2606:4700:4700::1111]:443/") == (
        "https://[2606:4700:4700::1111]/"
    )


@pytest.mark.parametrize("addresses", [
    ("8.8.8.8", "127.0.0.1"),
    ("2606:4700:4700::1111", "::1"),
])
def test_mixed_dns_records_block_before_tcp(monkeypatch, addresses):
    attempts = []

    def resolve(host, port, **kwargs):
        return [
            (socket.AF_INET6 if ":" in address else socket.AF_INET, socket.SOCK_STREAM, 6, "", (address, port))
            for address in addresses
        ]

    async def forbidden_connect(self, host, port, **kwargs):
        attempts.append((host, port))
        raise AssertionError("must not attempt any connection")

    monkeypatch.setattr(socket, "getaddrinfo", resolve)
    monkeypatch.setattr(AnyIOBackend, "connect_tcp", forbidden_connect)
    with pytest.raises(UnsafeSourceURL, match="blocked network"):
        asyncio.run(fetch_public_url("http://mixed.invalid/"))
    assert attempts == []


def test_public_ipv6_dns_record_is_pinned_without_hostname_reresolution(monkeypatch):
    attempts = []
    lookups = []

    def resolve(host, port, **kwargs):
        lookups.append(host)
        return [(socket.AF_INET6, socket.SOCK_STREAM, 6, "", ("2606:4700:4700::1111", port, 0, 0))]

    async def blocked_external_connect(self, host, port, **kwargs):
        attempts.append((host, port))
        raise httpcore.ConnectError("test forbids external TCP")

    monkeypatch.setattr(socket, "getaddrinfo", resolve)
    monkeypatch.setattr(AnyIOBackend, "connect_tcp", blocked_external_connect)
    with pytest.raises(httpx.ConnectError, match="test forbids external TCP"):
        asyncio.run(fetch_public_url("http://ipv6.invalid:8080/path"))
    assert lookups == ["ipv6.invalid"]
    assert attempts == [("2606:4700:4700::1111", 8080)]


def test_https_connection_pins_ip_but_uses_original_host_and_verified_sni(monkeypatch, tmp_path):
    """Real local TLS handshake, cert validation and HTTP/1.1 bytes; no external TCP."""
    cert, key = tmp_path / "test.crt", tmp_path / "test.key"
    subprocess.run([
        "openssl", "req", "-x509", "-newkey", "rsa:2048", "-nodes",
        "-keyout", str(key), "-out", str(cert), "-days", "1",
        "-subj", "/CN=public.invalid", "-addext", "subjectAltName=DNS:public.invalid",
    ], check=True, capture_output=True)
    client_context = ssl.create_default_context(cafile=str(cert))
    monkeypatch.setattr(default_transport, "create_ssl_context", lambda **kwargs: client_context)
    real_extra_info = AnyIOStream.get_extra_info
    # The test routes validated public IP traffic to a local socket. Report the
    # simulated public peer so the independent post-connect check can complete.
    monkeypatch.setattr(
        AnyIOStream, "get_extra_info",
        lambda self, info: ("8.8.8.8", 443) if info == "server_addr" else real_extra_info(self, info),
    )
    server_context = ssl.SSLContext(ssl.PROTOCOL_TLS_SERVER)
    server_context.load_cert_chain(str(cert), str(key))
    sni_names = []
    server_context.set_servername_callback(lambda sock, name, context: sni_names.append(name))
    requests = []
    attempts = []

    async def run():
        async def serve(reader, writer):
            requests.append(await reader.readuntil(b"\r\n\r\n"))
            writer.write(b"HTTP/1.1 200 OK\r\nContent-Type: text/plain\r\nContent-Length: 2\r\n\r\nOK")
            await writer.drain()
            writer.close()

        server = await asyncio.start_server(serve, "127.0.0.1", 0, ssl=server_context)
        port = server.sockets[0].getsockname()[1]
        original_resolve = socket.getaddrinfo
        original_connect = AnyIOBackend.connect_tcp

        def resolve(host, requested_port, *args, **kwargs):
            if host in {"public.invalid", "wrong.invalid"}:
                return [(socket.AF_INET, socket.SOCK_STREAM, 6, "", ("8.8.8.8", requested_port))]
            return original_resolve(host, requested_port, *args, **kwargs)

        async def local_only_connect(self, host, requested_port, **kwargs):
            attempts.append((host, requested_port))
            if host != "8.8.8.8":
                raise httpcore.ConnectError("test forbids non-pinned or external TCP")
            return await original_connect(self, "127.0.0.1", port, **kwargs)

        monkeypatch.setattr(socket, "getaddrinfo", resolve)
        monkeypatch.setattr(AnyIOBackend, "connect_tcp", local_only_connect)
        try:
            data, final_url, content_type = await fetch_public_url(f"https://public.invalid:{port}/article")
            assert (data, final_url, content_type) == (
                b"OK", f"https://public.invalid:{port}/article", "text/plain",
            )
            with pytest.raises(httpx.ConnectError):
                await fetch_public_url(f"https://wrong.invalid:{port}/article")
        finally:
            server.close()
            await server.wait_closed()
        return port

    port = asyncio.run(run())
    assert attempts == [("8.8.8.8", port), ("8.8.8.8", port)]
    assert sni_names == ["public.invalid", "wrong.invalid"]
    assert len(requests) == 1  # Wrong-host certificate fails before HTTP bytes.
    assert requests[0].startswith(f"GET /article HTTP/1.1\r\nHost: public.invalid:{port}\r\n".encode())

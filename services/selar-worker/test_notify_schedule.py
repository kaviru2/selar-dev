"""Tests for the scheduled email-notice trigger (no network: MockTransport)."""

import httpx

import notify_schedule


def _client(handler):
    return httpx.Client(transport=httpx.MockTransport(handler))


def test_skips_without_configuration():
    assert notify_schedule.run_notifications(api_url="", secret="s") == {"skipped": "SELAR_API_URL not set"}
    assert notify_schedule.run_notifications(api_url="https://api.test", secret="") == {"skipped": "WORKER_TRIGGER_SECRET not set"}
    assert "https" in notify_schedule.run_notifications(api_url="http://api.test", secret="s")["skipped"]


def test_posts_with_secret_and_returns_report():
    seen = {}

    def handler(request: httpx.Request) -> httpx.Response:
        seen["url"] = str(request.url)
        seen["secret"] = request.headers.get("X-Selar-Worker-Secret")
        seen["method"] = request.method
        return httpx.Response(200, json={"mode": "dry-run (nothing is sent)", "transport": "dry-run", "outcomes": {}})

    report = notify_schedule.run_notifications(api_url="https://api.test/", secret="s3cret", client=_client(handler))
    assert seen == {"url": "https://api.test/internal/notifications/run", "secret": "s3cret", "method": "POST"}
    assert report["transport"] == "dry-run"


def test_http_errors_do_not_raise():
    report = notify_schedule.run_notifications(api_url="https://api.test", secret="s", client=_client(lambda r: httpx.Response(401)))
    assert report == {"error": "HTTP 401"}

    def boom(request):
        raise httpx.ConnectError("down", request=request)

    assert notify_schedule.run_notifications(api_url="https://api.test", secret="s", client=_client(boom)) == {"error": "ConnectError"}

import importlib.util
from contextlib import contextmanager
from pathlib import Path

import httpx
import pytest


def load_e2e_demo_module():
    module_path = Path(__file__).resolve().parents[1] / "examples" / "e2e_demo.py"
    spec = importlib.util.spec_from_file_location("continua_e2e_demo", module_path)
    assert spec is not None
    assert spec.loader is not None
    module = importlib.util.module_from_spec(spec)
    spec.loader.exec_module(module)
    return module


class FakeHTTPClient:
    def __init__(self, requests, response):
        self.requests = requests
        self.response = response

    def __enter__(self):
        return self

    def __exit__(self, *_exc):
        return False

    def get(self, url, **kwargs):
        self.requests.append((url, kwargs))
        return self.response

    def post(self, url, **kwargs):
        self.requests.append((url, kwargs))
        return self.response


def test_resolve_api_key_uses_only_explicit_key(monkeypatch):
    demo = load_e2e_demo_module()
    requests = []

    demo.API_URL = "http://localhost:8080"
    demo.API_KEY = "explicit-key"
    monkeypatch.setattr(
        demo.httpx,
        "Client",
        lambda **_kwargs: FakeHTTPClient(
            requests,
            httpx.Response(401, json={"code": "invalid_api_key"}),
        ),
    )

    with pytest.raises(RuntimeError, match="CONTINUA_API_KEY"):
        demo.resolve_api_key()

    assert len(requests) == 1
    assert requests[0][0] == "http://localhost:8080/api/projects"
    assert requests[0][1]["headers"] == {"X-API-Key": "explicit-key"}


def test_resolve_api_key_bootstraps_local_project_when_key_missing(monkeypatch):
    demo = load_e2e_demo_module()
    requests = []

    demo.API_URL = "http://localhost:8080"
    demo.API_KEY = ""
    demo.DEMO_RUN_ID = "ci"
    monkeypatch.setattr(
        demo.httpx,
        "Client",
        lambda **_kwargs: FakeHTTPClient(
            requests,
            httpx.Response(201, json={"api_key": "created-key"}),
        ),
    )

    assert demo.resolve_api_key() == "created-key"
    assert requests == [
        (
            "http://localhost:8080/api/projects",
            {"json": {"name": "e2e-demo-ci"}},
        )
    ]


def test_resolve_api_key_rejects_wrong_demo_project(monkeypatch):
    demo = load_e2e_demo_module()

    demo.API_URL = "http://localhost:8080"
    demo.API_KEY = "explicit-key"
    monkeypatch.setenv("CONTINUA_DEMO_PROJECT_ID", "demo-project")
    monkeypatch.setattr(
        demo.httpx,
        "Client",
        lambda **_kwargs: FakeHTTPClient(
            [],
            httpx.Response(200, json={"projects": [{"id": "other-project"}]}),
        ),
    )

    with pytest.raises(RuntimeError, match="CONTINUA_API_KEY"):
        demo.resolve_api_key()


def test_openai_agent_mode_requires_openai_key():
    demo = load_e2e_demo_module()

    demo.AGENT_MODE = "openai"
    demo.OPENAI_API_KEY = ""

    with pytest.raises(RuntimeError, match="OPENAI_API_KEY"):
        demo.require_real_agent_backend()


def test_offline_agent_mode_is_explicit_fallback():
    demo = load_e2e_demo_module()

    demo.AGENT_MODE = "offline"
    result = demo.call_agent_model(
        purpose="unit_test",
        messages=[{"role": "user", "content": "hello"}],
    )

    assert result.provider == "offline"
    assert result.model == "offline-demo-model"
    assert "unit_test" in result.content


def test_main_exits_nonzero_when_verification_fails(monkeypatch):
    demo = load_e2e_demo_module()

    class FakeSDKClient:
        def shutdown(self):
            return None

    @contextmanager
    def fake_session(_session_id):
        yield

    monkeypatch.setattr(demo, "resolve_api_key", lambda: "explicit-key")
    monkeypatch.setattr(demo.Continua, "init", lambda **_kwargs: FakeSDKClient())
    monkeypatch.setattr(demo, "session", fake_session)
    monkeypatch.setattr(demo, "run_research_agent", lambda _query: {"ok": True})
    monkeypatch.setattr(demo, "run_code_review_agent", lambda _code: {"ok": True})

    def fake_failing_agent():
        raise TimeoutError("expected failure")

    monkeypatch.setattr(demo, "run_failing_agent", fake_failing_agent)
    monkeypatch.setattr(demo, "verify_demo_data", lambda: False)
    monkeypatch.setattr(demo.time, "sleep", lambda _seconds: None)

    with pytest.raises(SystemExit) as exc:
        demo.main()

    assert exc.value.code == 1

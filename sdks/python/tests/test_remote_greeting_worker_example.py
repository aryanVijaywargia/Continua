import importlib.util
from pathlib import Path
from types import ModuleType

import pytest

import continua


def load_remote_greeting_worker_module() -> ModuleType:
    module_path = (
        Path(__file__).resolve().parents[1]
        / "examples"
        / "remote_greeting_worker.py"
    )
    if not module_path.exists():
        pytest.fail(f"remote greeting worker example is missing: {module_path}")

    spec = importlib.util.spec_from_file_location(
        "continua_remote_greeting_worker", module_path
    )
    assert spec is not None
    assert spec.loader is not None
    module = importlib.util.module_from_spec(spec)
    spec.loader.exec_module(module)
    return module


def test_build_worker_registers_compose_greeting() -> None:
    module = load_remote_greeting_worker_module()

    worker = module.build_worker(
        api_key="test-key", endpoint="http://localhost:9"
    )
    try:
        assert isinstance(worker, continua.ActivityWorker)
        assert "examples.compose-greeting" in worker._handlers
    finally:
        worker.close()


def test_compose_greeting_returns_greeting() -> None:
    module = load_remote_greeting_worker_module()

    assert module.compose_greeting({"name": "Ada"}) == {
        "greeting": "hello, Ada"
    }


def test_compose_greeting_defaults_name() -> None:
    module = load_remote_greeting_worker_module()

    assert module.compose_greeting({}) == {"greeting": "hello, world"}

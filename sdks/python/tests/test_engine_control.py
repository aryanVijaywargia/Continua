"""Tests for the engine control client."""

from __future__ import annotations

from unittest.mock import MagicMock, patch

import pytest


def _run_response(status: str = "QUEUED", projection_state: str = "up_to_date") -> dict[str, object]:
    return {
        "run_id": "8fb17dc9-6565-4f3e-b671-8fd437416534",
        "instance_id": "f8a5bcbf-fc93-42fa-a2fb-e99bf76ed910",
        "instance_key": "instance-1",
        "definition_name": "checkout",
        "definition_version": "v1",
        "projection_state": projection_state,
        "status": status,
        "pending_work": {"pending_activity_tasks": 0, "pending_inbox_items": 0},
        "created_at": "2026-04-06T10:00:00Z",
        "updated_at": "2026-04-06T10:00:00Z",
        "result": None,
    }


def _result_response(status: str = "COMPLETED") -> dict[str, object]:
    return {
        "run_id": "8fb17dc9-6565-4f3e-b671-8fd437416534",
        "status": status,
        "result": {"ok": True} if status == "COMPLETED" else None,
        "failure": None if status == "COMPLETED" else {
            "error_code": "failed",
            "error_message": "boom",
            "status": status.lower(),
        },
    }


def _control_response(wake_applied: bool = True) -> dict[str, object]:
    return {
        "run_id": "8fb17dc9-6565-4f3e-b671-8fd437416534",
        "instance_key": "instance-1",
        "accepted": True,
        "wake_applied": wake_applied,
    }


def test_get_run_decodes_typed_response():
    with patch("continua.engine_control.httpx.Client") as mock_client_class:
        mock_client = MagicMock()
        mock_client.request.return_value = MagicMock(
            status_code=200,
            json=MagicMock(return_value=_run_response()),
        )
        mock_client_class.return_value = mock_client

        from continua import EngineControlClient
        from continua.types import EngineRunStatus

        client = EngineControlClient(api_key="test-key", endpoint="http://localhost:8080")
        response = client.get_run("8fb17dc9-6565-4f3e-b671-8fd437416534")

        assert response.instance_key == "instance-1"
        assert response.status == EngineRunStatus.QUEUED
        mock_client.request.assert_called_once_with(
            "GET",
            "/v1/engine/runs/8fb17dc9-6565-4f3e-b671-8fd437416534",
            json=None,
            params=None,
            headers=None,
        )


def test_start_sends_preview_header_and_decodes_response():
    with patch("continua.engine_control.httpx.Client") as mock_client_class:
        mock_client = MagicMock()
        mock_client.request.return_value = MagicMock(
            status_code=200,
            json=MagicMock(
                return_value={
                    "run_id": "8fb17dc9-6565-4f3e-b671-8fd437416534",
                    "instance_id": "f8a5bcbf-fc93-42fa-a2fb-e99bf76ed910",
                    "instance_key": "instance-1",
                    "definition_name": "checkout",
                    "definition_version": "v1",
                    "projection_state": "up_to_date",
                    "trace_id": "engine:8fb17dc9-6565-4f3e-b671-8fd437416534",
                    "status": "QUEUED",
                    "pending_work": {"pending_activity_tasks": 0, "pending_inbox_items": 0},
                    "created_at": "2026-04-06T10:00:00Z",
                    "updated_at": "2026-04-06T10:00:00Z",
                }
            ),
        )
        mock_client_class.return_value = mock_client

        from continua import EngineControlClient

        client = EngineControlClient(api_key="test-key", endpoint="http://localhost:8080")
        response = client.start(
            {
                "definition_name": "checkout",
                "definition_version": "v1",
                "instance_key": "instance-1",
                "request_key": "request-1",
            }
        )

        assert response.instance_key == "instance-1"
        mock_client.request.assert_called_once_with(
            "POST",
            "/v1/engine/runs",
            json={
                "definition_name": "checkout",
                "definition_version": "v1",
                "instance_key": "instance-1",
                "request_key": "request-1",
            },
            params=None,
            headers={"X-Continua-Engine-Preview": "1"},
        )


@pytest.mark.parametrize(
    ("method_name", "args", "path", "response_body", "expected_headers", "expected_json", "expected_params"),
    [
        (
            "signal",
            ("8fb17dc9-6565-4f3e-b671-8fd437416534", {"signal_name": "approval", "payload": {"ok": True}}),
            "/v1/engine/runs/8fb17dc9-6565-4f3e-b671-8fd437416534/signal",
            _control_response(True),
            {"X-Continua-Engine-Preview": "1"},
            {"signal_name": "approval", "payload": {"ok": True}},
            None,
        ),
        (
            "cancel",
            ("8fb17dc9-6565-4f3e-b671-8fd437416534",),
            "/v1/engine/runs/8fb17dc9-6565-4f3e-b671-8fd437416534/cancel",
            _control_response(False),
            {"X-Continua-Engine-Preview": "1"},
            None,
            None,
        ),
        (
            "terminate",
            ("8fb17dc9-6565-4f3e-b671-8fd437416534",),
            "/v1/engine/runs/8fb17dc9-6565-4f3e-b671-8fd437416534/terminate",
            _result_response("TERMINATED"),
            {"X-Continua-Engine-Preview": "1"},
            None,
            None,
        ),
        (
            "get_instance",
            ("instance-1",),
            "/v1/engine/instances/instance-1",
            {
                "instance_id": "f8a5bcbf-fc93-42fa-a2fb-e99bf76ed910",
                "instance_key": "instance-1",
                "definition_name": "checkout",
                "status": "ACTIVE",
                "current_run": _run_response(),
                "created_at": "2026-04-06T10:00:00Z",
                "updated_at": "2026-04-06T10:00:00Z",
            },
            None,
            None,
            None,
        ),
        (
            "get_pending_work",
            ("8fb17dc9-6565-4f3e-b671-8fd437416534",),
            "/v1/engine/runs/8fb17dc9-6565-4f3e-b671-8fd437416534/pending-work",
            {
                "run_id": "8fb17dc9-6565-4f3e-b671-8fd437416534",
                "current_wait": None,
                "activities": [],
                "timers": [],
                "signals": [],
                "pending_activity_tasks": 0,
                "pending_inbox_items": 0,
            },
            None,
            None,
            None,
        ),
    ],
)
def test_control_methods_use_expected_paths_and_headers(
    method_name: str,
    args: tuple[object, ...],
    path: str,
    response_body: dict[str, object],
    expected_headers: dict[str, str] | None,
    expected_json: dict[str, object] | None,
    expected_params: dict[str, object] | None,
):
    with patch("continua.engine_control.httpx.Client") as mock_client_class:
        mock_client = MagicMock()
        mock_client.request.return_value = MagicMock(
            status_code=200,
            json=MagicMock(return_value=response_body),
        )
        mock_client_class.return_value = mock_client

        from continua import EngineControlClient

        client = EngineControlClient(api_key="test-key", endpoint="http://localhost:8080")
        method = getattr(client, method_name)
        method(*args)

        mock_client.request.assert_called_once_with(
            "GET" if method_name.startswith("get_") else "POST",
            path,
            json=expected_json,
            params=expected_params,
            headers=expected_headers,
        )


def test_purge_sends_preview_header_and_body():
    with patch("continua.engine_control.httpx.Client") as mock_client_class:
        mock_client = MagicMock()
        mock_client.request.return_value = MagicMock(
            status_code=200,
            json=MagicMock(
                return_value={
                    "run_id": "8fb17dc9-6565-4f3e-b671-8fd437416534",
                    "mode": "projection_only",
                    "projection_state": "summary_only",
                    "deleted": True,
                }
            ),
        )
        mock_client_class.return_value = mock_client

        from continua import EngineControlClient

        client = EngineControlClient(api_key="test-key", endpoint="http://localhost:8080")
        response = client.purge(
            "8fb17dc9-6565-4f3e-b671-8fd437416534",
            mode="projection_only",
        )

        assert response.deleted is True
        mock_client.request.assert_called_once_with(
            "POST",
            "/v1/engine/runs/8fb17dc9-6565-4f3e-b671-8fd437416534/purge",
            json={"mode": "projection_only"},
            params=None,
            headers={"X-Continua-Engine-Preview": "1"},
        )


def test_purge_decodes_idempotent_response():
    with patch("continua.engine_control.httpx.Client") as mock_client_class:
        mock_client = MagicMock()
        mock_client.request.return_value = MagicMock(
            status_code=200,
            json=MagicMock(
                return_value={
                    "run_id": "8fb17dc9-6565-4f3e-b671-8fd437416534",
                    "mode": "projection_only",
                    "projection_state": "summary_only",
                    "deleted": False,
                }
            ),
        )
        mock_client_class.return_value = mock_client

        from continua import EngineControlClient
        from continua.types import EngineProjectionState

        client = EngineControlClient(api_key="test-key", endpoint="http://localhost:8080")
        response = client.purge(
            "8fb17dc9-6565-4f3e-b671-8fd437416534",
            mode="projection_only",
        )

        assert response.deleted is False
        assert response.projection_state == EngineProjectionState.summary_only


def test_get_history_decodes_expired_marker_and_params():
    with patch("continua.engine_control.httpx.Client") as mock_client_class:
        mock_client = MagicMock()
        mock_client.request.return_value = MagicMock(
            status_code=200,
            json=MagicMock(
                return_value={
                    "expired": True,
                    "events": [],
                    "has_more": False,
                }
            ),
        )
        mock_client_class.return_value = mock_client

        from continua import EngineControlClient

        client = EngineControlClient(api_key="test-key", endpoint="http://localhost:8080")
        response = client.get_history(
            "8fb17dc9-6565-4f3e-b671-8fd437416534",
            after=10,
            limit=50,
        )

        assert response.expired is True
        mock_client.request.assert_called_once_with(
            "GET",
            "/v1/engine/runs/8fb17dc9-6565-4f3e-b671-8fd437416534/history",
            json=None,
            params={"after": 10, "limit": 50},
            headers=None,
        )


def test_repair_decodes_reason_and_projection_state():
    with patch("continua.engine_control.httpx.Client") as mock_client_class:
        mock_client = MagicMock()
        mock_client.request.return_value = MagicMock(
            status_code=200,
            json=MagicMock(
                return_value={
                    "run_id": "8fb17dc9-6565-4f3e-b671-8fd437416534",
                    "accepted": True,
                    "reason": "repair_requested",
                    "projection_state": "catching_up",
                }
            ),
        )
        mock_client_class.return_value = mock_client

        from continua import EngineControlClient
        from continua.types import EngineProjectionState, EngineRepairReason

        client = EngineControlClient(api_key="test-key", endpoint="http://localhost:8080")
        response = client.repair("8fb17dc9-6565-4f3e-b671-8fd437416534")

        assert response.reason == EngineRepairReason.repair_requested
        assert response.projection_state == EngineProjectionState.catching_up


@pytest.mark.parametrize(
    ("reason", "accepted", "projection_state"),
    [
        ("already_up_to_date", False, "up_to_date"),
        ("history_expired", False, "journal_expired"),
        ("repair_requested", True, "catching_up"),
        ("already_catching_up", True, "catching_up"),
        ("no_events_to_project", False, "summary_only"),
    ],
)
def test_repair_decodes_all_supported_reasons(
    reason: str,
    accepted: bool,
    projection_state: str,
):
    with patch("continua.engine_control.httpx.Client") as mock_client_class:
        mock_client = MagicMock()
        mock_client.request.return_value = MagicMock(
            status_code=200,
            json=MagicMock(
                return_value={
                    "run_id": "8fb17dc9-6565-4f3e-b671-8fd437416534",
                    "accepted": accepted,
                    "reason": reason,
                    "projection_state": projection_state,
                }
            ),
        )
        mock_client_class.return_value = mock_client

        from continua import EngineControlClient
        from continua.types import EngineProjectionState, EngineRepairReason

        client = EngineControlClient(api_key="test-key", endpoint="http://localhost:8080")
        response = client.repair("8fb17dc9-6565-4f3e-b671-8fd437416534")

        assert response.reason == EngineRepairReason(reason)
        assert response.accepted is accepted
        assert response.projection_state == EngineProjectionState(projection_state)


def test_purge_maps_run_not_terminal_to_typed_error():
    with patch("continua.engine_control.httpx.Client") as mock_client_class:
        mock_client = MagicMock()
        mock_client.request.return_value = MagicMock(
            status_code=409,
            json=MagicMock(return_value={"code": "run_not_terminal", "message": "not terminal"}),
            text="not terminal",
        )
        mock_client_class.return_value = mock_client

        from continua import EngineControlClient
        from continua.exceptions import EngineRunNotTerminalError

        client = EngineControlClient(api_key="test-key", endpoint="http://localhost:8080")

        with pytest.raises(EngineRunNotTerminalError, match="not terminal"):
            client.purge("8fb17dc9-6565-4f3e-b671-8fd437416534", mode="full")


def test_get_result_maps_run_not_terminal_to_typed_error():
    with patch("continua.engine_control.httpx.Client") as mock_client_class:
        mock_client = MagicMock()
        mock_client.request.return_value = MagicMock(
            status_code=409,
            json=MagicMock(return_value={"code": "run_not_terminal", "message": "not yet"}),
            text="not yet",
        )
        mock_client_class.return_value = mock_client

        from continua import EngineControlClient
        from continua.exceptions import EngineRunNotTerminalError

        client = EngineControlClient(api_key="test-key", endpoint="http://localhost:8080")

        with pytest.raises(EngineRunNotTerminalError, match="not yet"):
            client.get_result("8fb17dc9-6565-4f3e-b671-8fd437416534")


def test_get_run_maps_not_found_to_typed_error():
    with patch("continua.engine_control.httpx.Client") as mock_client_class:
        mock_client = MagicMock()
        mock_client.request.return_value = MagicMock(
            status_code=404,
            json=MagicMock(return_value={"code": "not_found", "message": "missing"}),
            text="missing",
        )
        mock_client_class.return_value = mock_client

        from continua import EngineControlClient
        from continua.exceptions import EngineRunNotFoundError

        client = EngineControlClient(api_key="test-key", endpoint="http://localhost:8080")

        with pytest.raises(EngineRunNotFoundError, match="missing"):
            client.get_run("8fb17dc9-6565-4f3e-b671-8fd437416534")


def test_wait_for_terminal_returns_terminal_result():
    with patch("continua.engine_control.httpx.Client") as mock_client_class:
        with patch("continua.engine_control.time.sleep") as mock_sleep:
            mock_client = MagicMock()
            mock_client.request.side_effect = [
                MagicMock(
                    status_code=409,
                    json=MagicMock(return_value={"code": "run_not_terminal", "message": "pending"}),
                    text="pending",
                ),
                MagicMock(
                    status_code=200,
                    json=MagicMock(return_value=_result_response()),
                ),
            ]
            mock_client_class.return_value = mock_client

            from continua import EngineControlClient
            from continua.types import EngineRunStatus

            client = EngineControlClient(api_key="test-key", endpoint="http://localhost:8080")
            response = client.wait_for_terminal(
                "8fb17dc9-6565-4f3e-b671-8fd437416534",
                timeout=5.0,
                poll_interval=0.01,
            )

            assert response.status == EngineRunStatus.COMPLETED
            assert mock_client.request.call_count == 2
            mock_sleep.assert_called_once_with(0.01)


def test_wait_for_terminal_times_out():
    with patch("continua.engine_control.httpx.Client") as mock_client_class:
        with patch("continua.engine_control.time.sleep"):
            with patch(
                "continua.engine_control.time.monotonic",
                side_effect=[0.0, 0.0, 0.2],
            ):
                mock_client = MagicMock()
                mock_client.request.return_value = MagicMock(
                    status_code=409,
                    json=MagicMock(return_value={"code": "run_not_terminal", "message": "pending"}),
                    text="pending",
                )
                mock_client_class.return_value = mock_client

                from continua import EngineControlClient
                from continua.exceptions import EngineRunWaitTimeoutError

                client = EngineControlClient(api_key="test-key", endpoint="http://localhost:8080")

                with pytest.raises(EngineRunWaitTimeoutError, match="Timed out waiting"):
                    client.wait_for_terminal(
                        "8fb17dc9-6565-4f3e-b671-8fd437416534",
                        timeout=0.1,
                        poll_interval=0.01,
                    )


def test_wait_for_terminal_zero_timeout_raises_immediately():
    with patch("continua.engine_control.httpx.Client") as mock_client_class:
        with patch("continua.engine_control.time.sleep") as mock_sleep:
            with patch("continua.engine_control.time.monotonic", return_value=0.0):
                mock_client = MagicMock()
                mock_client.request.return_value = MagicMock(
                    status_code=409,
                    json=MagicMock(return_value={"code": "run_not_terminal", "message": "pending"}),
                    text="pending",
                )
                mock_client_class.return_value = mock_client

                from continua import EngineControlClient
                from continua.exceptions import EngineRunWaitTimeoutError

                client = EngineControlClient(api_key="test-key", endpoint="http://localhost:8080")

                with pytest.raises(EngineRunWaitTimeoutError):
                    client.wait_for_terminal(
                        "8fb17dc9-6565-4f3e-b671-8fd437416534",
                        timeout=0.0,
                        poll_interval=0.01,
                    )

                mock_sleep.assert_not_called()


@pytest.mark.parametrize("status", ["COMPLETED", "FAILED", "CANCELLED", "TERMINATED"])
def test_wait_for_terminal_returns_each_terminal_status(status: str):
    with patch("continua.engine_control.httpx.Client") as mock_client_class:
        mock_client = MagicMock()
        mock_client.request.return_value = MagicMock(
            status_code=200,
            json=MagicMock(return_value=_result_response(status)),
        )
        mock_client_class.return_value = mock_client

        from continua import EngineControlClient
        from continua.types import EngineRunStatus

        client = EngineControlClient(api_key="test-key", endpoint="http://localhost:8080")
        response = client.wait_for_terminal("8fb17dc9-6565-4f3e-b671-8fd437416534")

        assert response.status == EngineRunStatus(status)

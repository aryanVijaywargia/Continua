"""Tests for the engine control client."""

from __future__ import annotations

from unittest.mock import MagicMock, patch

import pytest

RUN_ID = "8fb17dc9-6565-4f3e-b671-8fd437416534"
NEXT_RUN_ID = "ca66395e-bcf5-4bab-83e1-84d4520f9463"
FINAL_RUN_ID = "6ff13fd6-5025-46d7-aa8d-6b07fe8f5587"


def _run_response(
    status: str = "QUEUED",
    projection_state: str = "up_to_date",
    *,
    run_id: str = RUN_ID,
) -> dict[str, object]:
    return {
        "run_id": run_id,
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


def _result_response(
    status: str = "COMPLETED",
    *,
    run_id: str = RUN_ID,
    continued_from_run_id: str | None = None,
    continued_to_run_id: str | None = None,
) -> dict[str, object]:
    failure = None
    if status not in {"COMPLETED", "CONTINUED_AS_NEW"}:
        failure = {
            "error_code": "failed",
            "error_message": "boom",
            "status": status.lower(),
        }

    return {
        "run_id": run_id,
        "continued_from_run_id": continued_from_run_id,
        "continued_to_run_id": continued_to_run_id,
        "continued_from_trace_id": (
            f"engine:{continued_from_run_id}" if continued_from_run_id is not None else None
        ),
        "continued_to_trace_id": (
            f"engine:{continued_to_run_id}" if continued_to_run_id is not None else None
        ),
        "status": status,
        "result": {"ok": True} if status == "COMPLETED" else None,
        "failure": failure,
    }


def _control_response(wake_applied: bool = True) -> dict[str, object]:
    return {
        "run_id": RUN_ID,
        "instance_key": "instance-1",
        "accepted": True,
        "wake_applied": wake_applied,
    }


def _backfill_response(
    *,
    dry_run: bool,
    limit: int,
    start_index: int,
    count: int,
    action: str,
    reason: str | None = None,
) -> dict[str, object]:
    results = []
    for index in range(start_index, start_index + count):
        result: dict[str, object] = {
            "run_id": f"00000000-0000-0000-0000-{index:012d}",
            "trace_id": f"engine:trace-{index}",
            "projection_state": "summary_only" if action == "would_repair" else "catching_up",
            "action": action,
        }
        if reason is not None:
            result["reason"] = reason
        results.append(result)

    repair_requested_count = count if action == "repair_requested" else 0
    skipped_count = count if action == "skipped" else 0

    return {
        "dry_run": dry_run,
        "limit": limit,
        "eligible_count": count,
        "repair_requested_count": repair_requested_count,
        "skipped_count": skipped_count,
        "results": results,
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
        response = client.get_run(RUN_ID)

        assert response.instance_key == "instance-1"
        assert response.status == EngineRunStatus.QUEUED
        mock_client.request.assert_called_once_with(
            "GET",
            f"/v1/engine/runs/{RUN_ID}",
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
                    "run_id": RUN_ID,
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
    (
        "method_name",
        "args",
        "path",
        "response_body",
        "expected_headers",
        "expected_json",
        "expected_params",
    ),
    [
        (
            "signal",
            (RUN_ID, {"signal_name": "approval", "payload": {"ok": True}}),
            f"/v1/engine/runs/{RUN_ID}/signal",
            _control_response(True),
            {"X-Continua-Engine-Preview": "1"},
            {"signal_name": "approval", "payload": {"ok": True}},
            None,
        ),
        (
            "cancel",
            (RUN_ID,),
            f"/v1/engine/runs/{RUN_ID}/cancel",
            _control_response(False),
            {"X-Continua-Engine-Preview": "1"},
            None,
            None,
        ),
        (
            "suspend",
            (RUN_ID,),
            f"/v1/engine/runs/{RUN_ID}/suspend",
            _run_response("SUSPENDED"),
            {"X-Continua-Engine-Preview": "1"},
            None,
            None,
        ),
        (
            "resume",
            (RUN_ID,),
            f"/v1/engine/runs/{RUN_ID}/resume",
            _run_response("QUEUED"),
            {"X-Continua-Engine-Preview": "1"},
            None,
            None,
        ),
        (
            "terminate",
            (RUN_ID,),
            f"/v1/engine/runs/{RUN_ID}/terminate",
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
            (RUN_ID,),
            f"/v1/engine/runs/{RUN_ID}/pending-work",
            {
                "run_id": RUN_ID,
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
                    "run_id": RUN_ID,
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
            RUN_ID,
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


def test_backfill_projections_decodes_typed_response_and_sends_preview_header():
    with patch("continua.engine_control.httpx.Client") as mock_client_class:
        mock_client = MagicMock()
        mock_client.request.return_value = MagicMock(
            status_code=200,
            json=MagicMock(
                return_value=_backfill_response(
                    dry_run=True,
                    limit=10,
                    start_index=1,
                    count=2,
                    action="would_repair",
                )
            ),
        )
        mock_client_class.return_value = mock_client

        from continua import EngineControlClient
        from continua.types import EngineProjectionBackfillAction

        client = EngineControlClient(api_key="test-key", endpoint="http://localhost:8080")
        response = client.backfill_projections(dry_run=True, limit=10)

        assert response.dry_run is True
        assert response.eligible_count == 2
        assert response.results[0].action == EngineProjectionBackfillAction.would_repair
        mock_client.request.assert_called_once_with(
            "POST",
            "/v1/engine/projections/backfill",
            json={"dry_run": True, "limit": 10},
            params=None,
            headers={"X-Continua-Engine-Preview": "1"},
        )


def test_backfill_projections_all_rejects_dry_run_paging():
    with patch("continua.engine_control.httpx.Client") as mock_client_class:
        mock_client = MagicMock()
        mock_client_class.return_value = mock_client

        from continua import EngineControlClient

        client = EngineControlClient(api_key="test-key", endpoint="http://localhost:8080")

        with pytest.raises(ValueError, match="cannot page dry-run previews"):
            client.backfill_projections_all(dry_run=True, max_total=100, limit=50)

        mock_client.request.assert_not_called()


def test_backfill_projections_all_stops_at_max_total():
    with patch("continua.engine_control.httpx.Client") as mock_client_class:
        mock_client = MagicMock()
        mock_client.request.side_effect = [
            MagicMock(
                status_code=200,
                json=MagicMock(
                    return_value=_backfill_response(
                        dry_run=False,
                        limit=50,
                        start_index=1,
                        count=50,
                        action="repair_requested",
                        reason="repair_requested",
                    )
                ),
            ),
            MagicMock(
                status_code=200,
                json=MagicMock(
                    return_value=_backfill_response(
                        dry_run=False,
                        limit=50,
                        start_index=51,
                        count=50,
                        action="repair_requested",
                        reason="repair_requested",
                    )
                ),
            ),
        ]
        mock_client_class.return_value = mock_client

        from continua import EngineControlClient

        client = EngineControlClient(api_key="test-key", endpoint="http://localhost:8080")
        response = client.backfill_projections_all(max_total=100, limit=50)

        assert response.eligible_count == 100
        assert response.repair_requested_count == 100
        assert response.skipped_count == 0
        assert len(response.results) == 100
        assert mock_client.request.call_count == 2


def test_backfill_projections_all_stops_when_eligible_runs_are_exhausted():
    with patch("continua.engine_control.httpx.Client") as mock_client_class:
        mock_client = MagicMock()
        mock_client.request.return_value = MagicMock(
            status_code=200,
            json=MagicMock(
                return_value=_backfill_response(
                    dry_run=False,
                    limit=50,
                    start_index=1,
                    count=30,
                    action="repair_requested",
                    reason="repair_requested",
                )
            ),
        )
        mock_client_class.return_value = mock_client

        from continua import EngineControlClient

        client = EngineControlClient(api_key="test-key", endpoint="http://localhost:8080")
        response = client.backfill_projections_all(max_total=1000, limit=50)

        assert response.eligible_count == 30
        assert response.repair_requested_count == 30
        assert len(response.results) == 30
        assert mock_client.request.call_count == 1


def test_backfill_projections_all_returns_aggregated_results():
    with patch("continua.engine_control.httpx.Client") as mock_client_class:
        mock_client = MagicMock()
        mock_client.request.side_effect = [
            MagicMock(
                status_code=200,
                json=MagicMock(
                    return_value=_backfill_response(
                        dry_run=False,
                        limit=2,
                        start_index=1,
                        count=2,
                        action="repair_requested",
                        reason="repair_requested",
                    )
                ),
            ),
            MagicMock(
                status_code=200,
                json=MagicMock(
                    return_value=_backfill_response(
                        dry_run=False,
                        limit=2,
                        start_index=3,
                        count=2,
                        action="skipped",
                        reason="already_catching_up",
                    )
                ),
            ),
            MagicMock(
                status_code=200,
                json=MagicMock(
                    return_value=_backfill_response(
                        dry_run=False,
                        limit=1,
                        start_index=5,
                        count=1,
                        action="repair_requested",
                        reason="repair_requested",
                    )
                ),
            ),
        ]
        mock_client_class.return_value = mock_client

        from continua import EngineControlClient

        client = EngineControlClient(api_key="test-key", endpoint="http://localhost:8080")
        response = client.backfill_projections_all(max_total=5, limit=2)

        assert response.eligible_count == 5
        assert response.repair_requested_count == 3
        assert response.skipped_count == 2
        assert [item.trace_id for item in response.results] == [
            "engine:trace-1",
            "engine:trace-2",
            "engine:trace-3",
            "engine:trace-4",
            "engine:trace-5",
        ]
        assert mock_client.request.call_args_list[-1].kwargs["json"]["limit"] == 1


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
            client.get_result(RUN_ID)


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
            client.get_run(RUN_ID)


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
                RUN_ID,
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
                        RUN_ID,
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
                        RUN_ID,
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
        response = client.wait_for_terminal(RUN_ID)

        assert response.status == EngineRunStatus(status)


def test_wait_for_terminal_default_does_not_follow_continuations():
    with patch("continua.engine_control.httpx.Client") as mock_client_class:
        mock_client = MagicMock()
        mock_client.request.return_value = MagicMock(
            status_code=200,
            json=MagicMock(
                return_value=_result_response(
                    "CONTINUED_AS_NEW",
                    run_id=RUN_ID,
                    continued_to_run_id=NEXT_RUN_ID,
                )
            ),
        )
        mock_client_class.return_value = mock_client

        from continua import EngineControlClient
        from continua.types import EngineRunStatus

        client = EngineControlClient(api_key="test-key", endpoint="http://localhost:8080")
        response = client.wait_for_terminal(RUN_ID)

        assert response.status == EngineRunStatus.CONTINUED_AS_NEW
        assert str(response.continued_to_run_id) == NEXT_RUN_ID
        assert mock_client.request.call_count == 1


def test_wait_for_terminal_follows_single_continuation():
    with patch("continua.engine_control.httpx.Client") as mock_client_class:
        with patch("continua.engine_control.time.sleep") as mock_sleep:
            mock_client = MagicMock()
            mock_client.request.side_effect = [
                MagicMock(
                    status_code=200,
                    json=MagicMock(
                        return_value=_result_response(
                            "CONTINUED_AS_NEW",
                            run_id=RUN_ID,
                            continued_to_run_id=NEXT_RUN_ID,
                        )
                    ),
                ),
                MagicMock(
                    status_code=409,
                    json=MagicMock(return_value={"code": "run_not_terminal", "message": "pending"}),
                    text="pending",
                ),
                MagicMock(
                    status_code=200,
                    json=MagicMock(return_value=_result_response("COMPLETED", run_id=NEXT_RUN_ID)),
                ),
            ]
            mock_client_class.return_value = mock_client

            from continua import EngineControlClient
            from continua.types import EngineRunStatus

            client = EngineControlClient(api_key="test-key", endpoint="http://localhost:8080")
            response = client.wait_for_terminal(
                RUN_ID,
                timeout=5.0,
                poll_interval=0.01,
                follow_continuations=True,
            )

            assert response.status == EngineRunStatus.COMPLETED
            assert str(response.run_id) == NEXT_RUN_ID
            assert [call.args[1] for call in mock_client.request.call_args_list] == [
                f"/v1/engine/runs/{RUN_ID}/result",
                f"/v1/engine/runs/{NEXT_RUN_ID}/result",
                f"/v1/engine/runs/{NEXT_RUN_ID}/result",
            ]
            mock_sleep.assert_called_once_with(0.01)


def test_wait_for_terminal_follows_multiple_continuations():
    with patch("continua.engine_control.httpx.Client") as mock_client_class:
        mock_client = MagicMock()
        mock_client.request.side_effect = [
            MagicMock(
                status_code=200,
                json=MagicMock(
                    return_value=_result_response(
                        "CONTINUED_AS_NEW",
                        run_id=RUN_ID,
                        continued_to_run_id=NEXT_RUN_ID,
                    )
                ),
            ),
            MagicMock(
                status_code=200,
                json=MagicMock(
                    return_value=_result_response(
                        "CONTINUED_AS_NEW",
                        run_id=NEXT_RUN_ID,
                        continued_from_run_id=RUN_ID,
                        continued_to_run_id=FINAL_RUN_ID,
                    )
                ),
            ),
            MagicMock(
                status_code=200,
                json=MagicMock(
                    return_value=_result_response(
                        "COMPLETED",
                        run_id=FINAL_RUN_ID,
                        continued_from_run_id=NEXT_RUN_ID,
                    )
                ),
            ),
        ]
        mock_client_class.return_value = mock_client

        from continua import EngineControlClient
        from continua.types import EngineRunStatus

        client = EngineControlClient(api_key="test-key", endpoint="http://localhost:8080")
        response = client.wait_for_terminal(RUN_ID, follow_continuations=True)

        assert response.status == EngineRunStatus.COMPLETED
        assert str(response.run_id) == FINAL_RUN_ID
        assert str(response.continued_from_run_id) == NEXT_RUN_ID


def test_wait_for_terminal_raises_when_continuation_depth_is_exceeded():
    with patch("continua.engine_control.httpx.Client") as mock_client_class:
        mock_client = MagicMock()
        mock_client.request.side_effect = [
            MagicMock(
                status_code=200,
                json=MagicMock(
                    return_value=_result_response(
                        "CONTINUED_AS_NEW",
                        run_id=RUN_ID,
                        continued_to_run_id=NEXT_RUN_ID,
                    )
                ),
            ),
            MagicMock(
                status_code=200,
                json=MagicMock(
                    return_value=_result_response(
                        "CONTINUED_AS_NEW",
                        run_id=NEXT_RUN_ID,
                        continued_from_run_id=RUN_ID,
                        continued_to_run_id=FINAL_RUN_ID,
                    )
                ),
            ),
        ]
        mock_client_class.return_value = mock_client

        from continua import EngineControlClient
        from continua.exceptions import EngineRunContinuationDepthError

        client = EngineControlClient(api_key="test-key", endpoint="http://localhost:8080")

        with pytest.raises(
            EngineRunContinuationDepthError,
            match="max_continuations=1",
        ):
            client.wait_for_terminal(
                RUN_ID,
                follow_continuations=True,
                max_continuations=1,
            )

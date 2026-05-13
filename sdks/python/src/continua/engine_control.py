"""Standalone engine control client for Continua."""

from __future__ import annotations

import time
from datetime import datetime
from typing import Any, TypeVar
from uuid import UUID

import httpx
from pydantic import BaseModel

from .exceptions import (
    AuthenticationError,
    EngineRunContinuationDepthError,
    EngineRunNotFoundError,
    EngineRunNotTerminalError,
    EngineRunWaitTimeoutError,
    NetworkError,
    ValidationError,
)
from .types import (
    EngineControlResponse,
    EngineInstanceResponse,
    EnginePendingWorkResponse,
    EngineProjectionBackfillRequest,
    EngineProjectionBackfillResponse,
    EngineProjectionBackfillRunResult,
    EngineProjectionState,
    EnginePurgeMode,
    EnginePurgeRequest,
    EnginePurgeResponse,
    EngineRepairResponse,
    EngineRunHistoryResponse,
    EngineRunResponse,
    EngineRunResultResponse,
    EngineRunStatus,
    EngineSignalRunRequest,
    EngineStartRunRequest,
    EngineStartRunResponse,
)

_PREVIEW_HEADERS = {"X-Continua-Engine-Preview": "1"}
ModelT = TypeVar("ModelT", bound=BaseModel)


class EngineControlClient:
    """Client for the engine control and read API surface."""

    def __init__(
        self,
        *,
        endpoint: str = "http://localhost:8080",
        api_key: str,
        timeout: float = 30.0,
        client: httpx.Client | None = None,
    ) -> None:
        self.endpoint = endpoint.rstrip("/")
        self.api_key = api_key
        self.timeout = timeout
        self._owns_client = client is None
        self._client = client or httpx.Client(
            base_url=self.endpoint,
            headers={"X-API-Key": api_key},
            timeout=timeout,
        )

    def close(self) -> None:
        if self._owns_client:
            self._client.close()

    def start(self, request: EngineStartRunRequest | dict[str, Any]) -> EngineStartRunResponse:
        return self._request_model(
            "POST",
            "/v1/engine/runs",
            EngineStartRunResponse,
            json=self._json_body(request),
            headers=_PREVIEW_HEADERS,
        )

    def signal(
        self,
        run_id: UUID | str,
        request: EngineSignalRunRequest | dict[str, Any],
    ) -> EngineControlResponse:
        return self._request_model(
            "POST",
            f"/v1/engine/runs/{run_id}/signal",
            EngineControlResponse,
            json=self._json_body(request),
            headers=_PREVIEW_HEADERS,
        )

    def cancel(self, run_id: UUID | str) -> EngineControlResponse:
        return self._request_model(
            "POST",
            f"/v1/engine/runs/{run_id}/cancel",
            EngineControlResponse,
            headers=_PREVIEW_HEADERS,
        )

    def suspend(self, run_id: UUID | str) -> EngineRunResponse:
        return self._request_model(
            "POST",
            f"/v1/engine/runs/{run_id}/suspend",
            EngineRunResponse,
            headers=_PREVIEW_HEADERS,
        )

    def resume(self, run_id: UUID | str) -> EngineRunResponse:
        return self._request_model(
            "POST",
            f"/v1/engine/runs/{run_id}/resume",
            EngineRunResponse,
            headers=_PREVIEW_HEADERS,
        )

    def terminate(self, run_id: UUID | str) -> EngineRunResultResponse:
        return self._request_model(
            "POST",
            f"/v1/engine/runs/{run_id}/terminate",
            EngineRunResultResponse,
            headers=_PREVIEW_HEADERS,
        )

    def get_instance(self, instance_key: str) -> EngineInstanceResponse:
        return self._request_model(
            "GET",
            f"/v1/engine/instances/{instance_key}",
            EngineInstanceResponse,
        )

    def get_run(self, run_id: UUID | str) -> EngineRunResponse:
        return self._request_model("GET", f"/v1/engine/runs/{run_id}", EngineRunResponse)

    def get_result(self, run_id: UUID | str) -> EngineRunResultResponse:
        return self._request_model(
            "GET",
            f"/v1/engine/runs/{run_id}/result",
            EngineRunResultResponse,
        )

    def get_history(
        self,
        run_id: UUID | str,
        *,
        after: int | None = None,
        limit: int | None = None,
    ) -> EngineRunHistoryResponse:
        params: dict[str, Any] = {}
        if after is not None:
            params["after"] = after
        if limit is not None:
            params["limit"] = limit
        return self._request_model(
            "GET",
            f"/v1/engine/runs/{run_id}/history",
            EngineRunHistoryResponse,
            params=params or None,
        )

    def get_pending_work(self, run_id: UUID | str) -> EnginePendingWorkResponse:
        return self._request_model(
            "GET",
            f"/v1/engine/runs/{run_id}/pending-work",
            EnginePendingWorkResponse,
        )

    def purge(
        self,
        run_id: UUID | str,
        *,
        mode: EnginePurgeMode | str,
    ) -> EnginePurgeResponse:
        request = EnginePurgeRequest.model_validate({"mode": mode})
        return self._request_model(
            "POST",
            f"/v1/engine/runs/{run_id}/purge",
            EnginePurgeResponse,
            json=request.model_dump(mode="json"),
            headers=_PREVIEW_HEADERS,
        )

    def repair(self, run_id: UUID | str) -> EngineRepairResponse:
        return self._request_model(
            "POST",
            f"/v1/engine/runs/{run_id}/repair",
            EngineRepairResponse,
            headers=_PREVIEW_HEADERS,
        )

    def backfill_projections(
        self,
        *,
        dry_run: bool = False,
        limit: int | None = None,
        older_than: datetime | str | None = None,
        engine_instance_key: str | None = None,
        engine_definition_name: str | None = None,
        engine_run_status: EngineRunStatus | str | None = None,
        engine_projection_state: EngineProjectionState | str | None = None,
    ) -> EngineProjectionBackfillResponse:
        request = EngineProjectionBackfillRequest.model_validate(
            {
                "dry_run": dry_run,
                "limit": limit,
                "older_than": older_than,
                "engine_instance_key": engine_instance_key,
                "engine_definition_name": engine_definition_name,
                "engine_run_status": engine_run_status,
                "engine_projection_state": engine_projection_state,
            }
        )
        return self._request_model(
            "POST",
            "/v1/engine/projections/backfill",
            EngineProjectionBackfillResponse,
            json=request.model_dump(mode="json", exclude_none=True),
            headers=_PREVIEW_HEADERS,
        )

    def backfill_projections_all(
        self,
        *,
        max_total: int = 1000,
        dry_run: bool = False,
        limit: int | None = None,
        older_than: datetime | str | None = None,
        engine_instance_key: str | None = None,
        engine_definition_name: str | None = None,
        engine_run_status: EngineRunStatus | str | None = None,
        engine_projection_state: EngineProjectionState | str | None = None,
    ) -> EngineProjectionBackfillResponse:
        if dry_run:
            raise ValueError(
                "backfill_projections_all() cannot page dry-run previews because "
                "the backfill API has no cursor and dry-run calls do not mutate "
                "eligibility; call backfill_projections(dry_run=True, limit=...) "
                "for a bounded preview."
            )

        page_limit = limit or 50
        remaining = max_total
        aggregate_results: list[EngineProjectionBackfillRunResult] = []
        aggregate_repair_requested_count = 0
        aggregate_skipped_count = 0

        while remaining > 0:
            response = self.backfill_projections(
                dry_run=False,
                limit=min(page_limit, remaining),
                older_than=older_than,
                engine_instance_key=engine_instance_key,
                engine_definition_name=engine_definition_name,
                engine_run_status=engine_run_status,
                engine_projection_state=engine_projection_state,
            )
            aggregate_results.extend(response.results)
            aggregate_repair_requested_count += response.repair_requested_count
            aggregate_skipped_count += response.skipped_count
            remaining -= len(response.results)

            if len(response.results) < response.limit:
                break

        return EngineProjectionBackfillResponse(
            dry_run=dry_run,
            limit=page_limit,
            eligible_count=len(aggregate_results),
            repair_requested_count=aggregate_repair_requested_count,
            skipped_count=aggregate_skipped_count,
            results=aggregate_results,
        )

    def wait_for_terminal(
        self,
        run_id: UUID | str,
        *,
        timeout: float | None = None,
        poll_interval: float = 1.0,
        follow_continuations: bool = False,
        max_continuations: int = 32,
    ) -> EngineRunResultResponse:
        deadline = None if timeout is None else time.monotonic() + timeout
        current_run_id: UUID | str = run_id
        continuations_followed = 0

        while True:
            try:
                response = self.get_result(current_run_id)
            except EngineRunNotTerminalError:
                if deadline is not None and time.monotonic() >= deadline:
                    raise EngineRunWaitTimeoutError(str(current_run_id), timeout)
                time.sleep(poll_interval)
                continue

            if (
                follow_continuations
                and response.status == EngineRunStatus.CONTINUED_AS_NEW
                and response.continued_to_run_id is not None
            ):
                if continuations_followed >= max_continuations:
                    raise EngineRunContinuationDepthError(str(run_id), max_continuations)
                continuations_followed += 1
                current_run_id = response.continued_to_run_id
                continue

            return response

    def _request_model(
        self,
        method: str,
        path: str,
        model: type[ModelT],
        *,
        json: Any | None = None,
        params: dict[str, Any] | None = None,
        headers: dict[str, str] | None = None,
    ) -> ModelT:
        response = self._request(method, path, json=json, params=params, headers=headers)
        return model.model_validate(response.json())

    def _request(
        self,
        method: str,
        path: str,
        *,
        json: Any | None = None,
        params: dict[str, Any] | None = None,
        headers: dict[str, str] | None = None,
    ) -> httpx.Response:
        try:
            response = self._client.request(
                method,
                path,
                json=json,
                params=params,
                headers=headers,
            )
        except httpx.RequestError as exc:
            raise NetworkError("Network request failed", cause=exc) from exc

        if 200 <= response.status_code < 300:
            return response

        self._raise_for_response(response)
        raise AssertionError("unreachable")

    def _raise_for_response(self, response: httpx.Response) -> None:
        payload: dict[str, Any] = {}
        try:
            payload = response.json()
        except ValueError:
            payload = {}

        error_code = payload.get("code")
        error_message = payload.get("message") or response.text or "Request failed"

        if response.status_code == 401:
            raise AuthenticationError(error_message)
        if response.status_code == 404:
            raise EngineRunNotFoundError(error_message)
        if response.status_code == 409 and error_code == "run_not_terminal":
            raise EngineRunNotTerminalError(error_message)
        if response.status_code == 400:
            raise ValidationError("Validation error", error_message)
        raise NetworkError(
            f"Engine control request failed with status {response.status_code}",
        )

    @staticmethod
    def _json_body(value: BaseModel | dict[str, Any]) -> dict[str, Any]:
        if isinstance(value, BaseModel):
            return value.model_dump(mode="json", exclude_none=True)
        return value

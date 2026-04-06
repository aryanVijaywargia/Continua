"""Standalone engine control client for Continua."""

from __future__ import annotations

import time
from typing import Any
from uuid import UUID

import httpx
from pydantic import BaseModel

from .exceptions import (
    AuthenticationError,
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
    EnginePurgeMode,
    EnginePurgeRequest,
    EnginePurgeResponse,
    EngineRepairResponse,
    EngineRunHistoryResponse,
    EngineRunResponse,
    EngineRunResultResponse,
    EngineSignalRunRequest,
    EngineStartRunRequest,
    EngineStartRunResponse,
)

_PREVIEW_HEADERS = {"X-Continua-Engine-Preview": "1"}


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
        request = EnginePurgeRequest(mode=mode)
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

    def wait_for_terminal(
        self,
        run_id: UUID | str,
        *,
        timeout: float | None = None,
        poll_interval: float = 1.0,
    ) -> EngineRunResultResponse:
        deadline = None if timeout is None else time.monotonic() + timeout

        while True:
            try:
                return self.get_result(run_id)
            except EngineRunNotTerminalError:
                if deadline is not None and time.monotonic() >= deadline:
                    raise EngineRunWaitTimeoutError(str(run_id), timeout)
                time.sleep(poll_interval)

    def _request_model(
        self,
        method: str,
        path: str,
        model: type[BaseModel],
        *,
        json: Any | None = None,
        params: dict[str, Any] | None = None,
        headers: dict[str, str] | None = None,
    ) -> BaseModel:
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

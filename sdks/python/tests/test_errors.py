"""Tests for Python SDK error handling (Phase 3: Spec 6).

These tests verify custom exception handling as specified in
specs/python-sdk-polish/spec.md
"""

from unittest.mock import MagicMock, patch

import httpx
import pytest


class TestCustomExceptions:
    """Tests for custom exception types."""

    def test_authentication_error_on_401(self):
        """Scenario: Authentication error
        WHEN the API returns 401 Unauthorized
        THEN AuthenticationError is raised
        AND the error message indicates invalid API key
        """
        with patch("continua.client.httpx.Client") as mock_client_class:
            mock_client = MagicMock()
            mock_response = MagicMock()
            mock_response.status_code = 401
            mock_response.text = "Invalid API key"
            mock_response.raise_for_status.side_effect = httpx.HTTPStatusError(
                "401 Unauthorized",
                request=MagicMock(),
                response=mock_response,
            )
            mock_client.post.return_value = mock_response
            mock_client_class.return_value = mock_client

            from continua import Continua
            from continua.exceptions import AuthenticationError

            Continua._instance = None
            client = Continua(api_key="invalid-key", endpoint="http://localhost:8080")

            with pytest.raises(AuthenticationError) as exc_info:
                client._send_with_retry(
                    "/v1/ingest",
                    {"batch_key": "test", "traces": [{"trace_id": "t1", "name": "test"}]},
                )

            assert "invalid" in str(exc_info.value).lower() or "api key" in str(exc_info.value).lower()
            client._batch.shutdown()

    def test_rate_limit_error_on_429(self):
        """Scenario: Rate limit error
        WHEN the API returns 429 Too Many Requests
        THEN RateLimitError is raised
        AND the error includes retry-after information if available
        """
        with patch("continua.client.httpx.Client") as mock_client_class:
            mock_client = MagicMock()
            mock_response = MagicMock()
            mock_response.status_code = 429
            mock_response.headers = {"Retry-After": "60"}
            mock_response.raise_for_status.side_effect = httpx.HTTPStatusError(
                "429 Too Many Requests",
                request=MagicMock(),
                response=mock_response,
            )
            mock_client.post.return_value = mock_response
            mock_client_class.return_value = mock_client

            from continua import Continua
            from continua.exceptions import RateLimitError

            Continua._instance = None
            client = Continua(api_key="test-key", endpoint="http://localhost:8080")

            with pytest.raises(RateLimitError) as exc_info:
                client._send_with_retry(
                    "/v1/ingest",
                    {"batch_key": "test", "traces": [{"trace_id": "t1", "name": "test"}]},
                )

            # Error should include retry-after info
            error = exc_info.value
            assert hasattr(error, "retry_after") and error.retry_after == 60
            client._batch.shutdown()

    def test_validation_error_on_400(self):
        """Scenario: Validation error
        WHEN the API returns 400 Bad Request
        THEN ValidationError is raised
        AND the error message includes validation details
        """
        with patch("continua.client.httpx.Client") as mock_client_class:
            mock_client = MagicMock()
            mock_response = MagicMock()
            mock_response.status_code = 400
            mock_response.json.return_value = {
                "error": "validation_error",
                "message": "trace_id is required",
            }
            mock_response.raise_for_status.side_effect = httpx.HTTPStatusError(
                "400 Bad Request",
                request=MagicMock(),
                response=mock_response,
            )
            mock_client.post.return_value = mock_response
            mock_client_class.return_value = mock_client

            from continua import Continua
            from continua.exceptions import ValidationError

            Continua._instance = None
            client = Continua(api_key="test-key", endpoint="http://localhost:8080")

            with pytest.raises(ValidationError) as exc_info:
                client._send_with_retry(
                    "/v1/ingest",
                    {"batch_key": "test", "traces": [{"name": "test"}]},
                )

            assert "trace_id" in str(exc_info.value).lower() or "validation" in str(exc_info.value).lower()
            client._batch.shutdown()

    def test_network_error_after_retries_exhausted(self):
        """Scenario: Network error
        WHEN the network request fails after retries
        THEN NetworkError is raised
        AND the error includes the number of retry attempts
        """
        with patch("continua.client.httpx.Client") as mock_client_class:
            with patch("continua.client.time.sleep"):
                mock_client = MagicMock()
                mock_client.post.side_effect = httpx.ConnectError("Connection refused")
                mock_client_class.return_value = mock_client

                from continua import Continua
                from continua.exceptions import NetworkError

                Continua._instance = None
                client = Continua(api_key="test-key", endpoint="http://localhost:8080")

                with pytest.raises(NetworkError) as exc_info:
                    client._send_with_retry(
                        "/v1/ingest",
                        {"batch_key": "test", "traces": [{"trace_id": "t1", "name": "test"}]},
                    )

                # Error should mention retry attempts
                assert "3" in str(exc_info.value) or "retries" in str(exc_info.value).lower()
                client._batch.shutdown()


class TestRetryWithBackoff:
    """Tests for retry behavior with exponential backoff."""

    def test_retry_on_connection_error(self):
        """Scenario: Retry on connection error
        WHEN a connection error occurs
        THEN the request is retried up to 3 times
        AND each retry waits (2^attempt + random jitter) seconds
        """
        with patch("continua.client.httpx.Client") as mock_client_class:
            with patch("continua.client.time.sleep") as mock_sleep:
                mock_client = MagicMock()
                # Fail twice, succeed on third attempt
                mock_response_success = MagicMock()
                mock_response_success.json.return_value = {"status": "ok", "batch_key": "test"}
                mock_client.post.side_effect = [
                    httpx.ConnectError("Connection refused"),
                    httpx.ConnectError("Connection refused"),
                    mock_response_success,
                ]
                mock_client_class.return_value = mock_client

                from continua import Continua

                Continua._instance = None
                client = Continua(api_key="test-key", endpoint="http://localhost:8080")

                # This should succeed after 2 retries
                client._send_batch(
                    traces=[{"trace_id": "t1", "name": "test"}],
                    spans=[],
                    events=[],
                )

                # Should have been called 3 times
                assert mock_client.post.call_count == 3

                # Should have slept twice (before retry 2 and 3)
                assert mock_sleep.call_count == 2

                # Check backoff delays (2^0 + jitter, 2^1 + jitter)
                first_delay = mock_sleep.call_args_list[0][0][0]
                second_delay = mock_sleep.call_args_list[1][0][0]
                assert 1 <= first_delay <= 2  # 2^0 + jitter (0-1)
                assert 2 <= second_delay <= 4  # 2^1 + jitter (0-1)

                client._batch.shutdown()

    def test_retry_on_timeout(self):
        """Scenario: Retry on timeout
        WHEN a request timeout occurs
        THEN the request is retried up to 3 times
        """
        with patch("continua.client.httpx.Client") as mock_client_class:
            with patch("continua.client.time.sleep"):
                mock_client = MagicMock()
                mock_response_success = MagicMock()
                mock_response_success.json.return_value = {"status": "ok", "batch_key": "test"}
                mock_client.post.side_effect = [
                    httpx.TimeoutException("Request timed out"),
                    mock_response_success,
                ]
                mock_client_class.return_value = mock_client

                from continua import Continua

                Continua._instance = None
                client = Continua(api_key="test-key", endpoint="http://localhost:8080")

                client._send_batch(
                    traces=[{"trace_id": "t1", "name": "test"}],
                    spans=[],
                    events=[],
                )

                assert mock_client.post.call_count == 2
                client._batch.shutdown()

    def test_no_retry_on_authentication_error(self):
        """Scenario: No retry on authentication error
        WHEN a 401 error occurs
        THEN no retry is attempted
        AND AuthenticationError is raised immediately
        """
        with patch("continua.client.httpx.Client") as mock_client_class:
            with patch("continua.client.time.sleep") as mock_sleep:
                mock_client = MagicMock()
                mock_response = MagicMock()
                mock_response.status_code = 401
                mock_response.raise_for_status.side_effect = httpx.HTTPStatusError(
                    "401 Unauthorized",
                    request=MagicMock(),
                    response=mock_response,
                )
                mock_client.post.return_value = mock_response
                mock_client_class.return_value = mock_client

                from continua import Continua
                from continua.exceptions import AuthenticationError

                Continua._instance = None
                client = Continua(api_key="test-key", endpoint="http://localhost:8080")

                with pytest.raises(AuthenticationError):
                    client._send_with_retry(
                        "/v1/ingest",
                        {"batch_key": "test", "traces": [{"trace_id": "t1", "name": "test"}]},
                    )

                # Should only be called once (no retries for auth errors)
                assert mock_client.post.call_count == 1
                mock_sleep.assert_not_called()
                client._batch.shutdown()

    def test_retry_exhaustion_raises_network_error(self):
        """Scenario: Retry exhaustion
        WHEN all retry attempts fail
        THEN NetworkError is raised
        AND the original error is preserved as cause
        """
        with patch("continua.client.httpx.Client") as mock_client_class:
            with patch("continua.client.time.sleep"):
                mock_client = MagicMock()
                original_error = httpx.ConnectError("Connection refused")
                mock_client.post.side_effect = original_error
                mock_client_class.return_value = mock_client

                from continua import Continua
                from continua.exceptions import NetworkError

                Continua._instance = None
                client = Continua(api_key="test-key", endpoint="http://localhost:8080")

                with pytest.raises(NetworkError) as exc_info:
                    client._send_with_retry(
                        "/v1/ingest",
                        {"batch_key": "test", "traces": [{"trace_id": "t1", "name": "test"}]},
                    )

                # Should have tried 4 times (initial + 3 retries)
                assert mock_client.post.call_count == 4

                # Original error should be preserved as cause
                assert exc_info.value.__cause__ is not None
                client._batch.shutdown()


class TestSessionContextManager:
    """Tests for session context manager."""

    def test_session_context_sets_session_id(self):
        """Scenario: Session context sets session_id
        WHEN code runs inside `with continua.session("sess_123"):`
        THEN all traces created inherit session_id="sess_123"
        """
        with patch("continua.client.httpx.Client") as mock_client_class:
            mock_client = MagicMock()
            mock_client_class.return_value = mock_client

            from continua import Continua, session, trace

            # Reset and create instance via init() so get_instance() finds it
            Continua._instance = None
            client = Continua.init(api_key="test-key", endpoint="http://localhost:8080")

            with session("sess_123"):

                @trace()
                def my_operation():
                    return "result"

                my_operation()

            # Check that the trace was created with session_id
            # Note: traces are added in pairs (start and end)
            assert len(client._batch._traces) >= 1
            assert client._batch._traces[0].get("session_id") == "sess_123"
            client.shutdown()

    def test_session_context_without_id_keeps_session_none(self):
        """Scenario: Session context without explicit ID
        WHEN code runs inside `with continua.session():` (no ID)
        THEN no session_id is attached to traces
        """
        with patch("continua.client.httpx.Client") as mock_client_class:
            mock_client = MagicMock()
            mock_client_class.return_value = mock_client

            from continua import Continua, session, trace

            Continua._instance = None
            client = Continua.init(api_key="test-key", endpoint="http://localhost:8080")

            with session() as sess:
                assert sess.session_id is None

                @trace()
                def my_operation():
                    return "result"

                my_operation()

            assert len(client._batch._traces) >= 1
            assert client._batch._traces[0].get("session_id") is None
            client.shutdown()

    def test_session_context_cleanup(self):
        """Scenario: Session context cleanup
        WHEN the session context exits
        THEN the session_id is cleared from context
        AND subsequent traces do not inherit the session
        """
        with patch("continua.client.httpx.Client") as mock_client_class:
            mock_client = MagicMock()
            mock_client_class.return_value = mock_client

            from continua import Continua, session, trace

            Continua._instance = None
            client = Continua.init(api_key="test-key", endpoint="http://localhost:8080")

            # Create trace inside session
            with session("sess_123"):

                @trace()
                def inside_session():
                    return "inside"

                inside_session()

            # Create trace outside session
            @trace()
            def outside_session():
                return "outside"

            outside_session()

            # First trace should have session_id, second should not
            assert len(client._batch._traces) >= 2
            # Find the start traces (ones with session_id or without)
            traces_with_session = [t for t in client._batch._traces if t.get("session_id") == "sess_123"]
            traces_without_session = [t for t in client._batch._traces if t.get("session_id") is None]
            assert len(traces_with_session) >= 1
            assert len(traces_without_session) >= 1
            client.shutdown()


class TestSpanHelperMethods:
    """Tests for span helper methods."""

    def test_set_llm_response(self):
        """Scenario: Set LLM response
        WHEN span.set_llm_response(model, messages, response, tokens_in, tokens_out) is called
        THEN the span is updated with model, input (messages), output (response), and token counts
        """
        with patch("continua.client.httpx.Client") as mock_client_class:
            mock_client = MagicMock()
            mock_client_class.return_value = mock_client

            from continua import Continua, trace
            from continua.span import span

            Continua._instance = None
            client = Continua.init(api_key="test-key", endpoint="http://localhost:8080")

            @trace()
            def my_agent():
                with span("llm_call", kind="llm") as s:
                    messages = [{"role": "user", "content": "Hello"}]
                    response = {"role": "assistant", "content": "Hi there!"}
                    s.set_llm_response(
                        model="gpt-4",
                        messages=messages,
                        response=response,
                        tokens_in=10,
                        tokens_out=5,
                    )

            my_agent()

            # Check span was created with LLM fields
            # There will be 2 span entries (start and end), check the last one
            assert len(client._batch._spans) >= 1
            span_data = client._batch._spans[-1]  # Get the final span data
            assert span_data.get("model") == "gpt-4"
            assert span_data.get("prompt_tokens") == 10
            assert span_data.get("completion_tokens") == 5
            assert span_data.get("total_tokens") is None
            assert span_data.get("input") == [{"role": "user", "content": "Hello"}]
            assert span_data.get("output") == {"role": "assistant", "content": "Hi there!"}
            client.shutdown()

    def test_set_tool_call(self):
        """Scenario: Set tool call
        WHEN span.set_tool_call(tool_name, arguments, result) is called
        THEN the span is updated with tool name, input (arguments), and output (result)
        """
        with patch("continua.client.httpx.Client") as mock_client_class:
            mock_client = MagicMock()
            mock_client_class.return_value = mock_client

            from continua import Continua, trace
            from continua.span import span

            Continua._instance = None
            client = Continua.init(api_key="test-key", endpoint="http://localhost:8080")

            @trace()
            def my_agent():
                with span("tool_call", kind="tool") as s:
                    s.set_tool_call(
                        tool_name="get_weather",
                        arguments={"city": "New York"},
                        result={"temperature": 72, "conditions": "sunny"},
                    )

            my_agent()

            assert len(client._batch._spans) >= 1
            span_data = client._batch._spans[-1]  # Get the final span data
            assert span_data.get("name") == "get_weather"
            assert span_data.get("input") == {"city": "New York"}
            assert span_data.get("output") == {"temperature": 72, "conditions": "sunny"}
            client.shutdown()

    def test_log_message(self):
        """Scenario: Log message
        WHEN span.log(message, level, payload) is called
        THEN an event is recorded on the span with the message, level, and optional payload
        """
        with patch("continua.client.httpx.Client") as mock_client_class:
            mock_client = MagicMock()
            mock_client_class.return_value = mock_client

            from continua import Continua, trace
            from continua.span import span

            Continua._instance = None
            client = Continua.init(api_key="test-key", endpoint="http://localhost:8080")

            @trace()
            def my_agent():
                with span("my_operation") as s:
                    s.log("Processing started", level="info")
                    s.log(
                        "Found items",
                        level="debug",
                        payload={"count": 42, "items": ["a", "b"]},
                    )

            my_agent()

            # Check events were created
            assert len(client._batch._events) == 2

            event1 = client._batch._events[0]
            assert event1.get("message") == "Processing started"
            assert event1.get("level") == "info"

            event2 = client._batch._events[1]
            assert event2.get("message") == "Found items"
            assert event2.get("level") == "debug"
            assert event2.get("payload") == {"count": 42, "items": ["a", "b"]}
            client.shutdown()

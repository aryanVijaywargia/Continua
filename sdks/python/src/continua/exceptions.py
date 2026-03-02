"""Custom exceptions for the Continua SDK."""

from __future__ import annotations


class ContinuaError(Exception):
    """Base exception for all Continua errors."""

    pass


class AuthenticationError(ContinuaError):
    """Raised when API authentication fails (401 Unauthorized).

    This error indicates an invalid or missing API key.
    """

    def __init__(self, message: str = "Invalid or missing API key") -> None:
        super().__init__(message)
        self.message = message


class RateLimitError(ContinuaError):
    """Raised when the API rate limit is exceeded (429 Too Many Requests).

    Attributes:
        retry_after: Number of seconds to wait before retrying, if available
    """

    def __init__(
        self,
        message: str = "Rate limit exceeded",
        retry_after: int | None = None,
    ) -> None:
        super().__init__(message)
        self.message = message
        self.retry_after = retry_after


class ValidationError(ContinuaError):
    """Raised when the API rejects a request due to validation errors (400 Bad Request).

    Attributes:
        details: Additional validation error details from the API
    """

    def __init__(
        self,
        message: str = "Validation error",
        details: str | None = None,
    ) -> None:
        full_message = f"{message}: {details}" if details else message
        super().__init__(full_message)
        self.message = message
        self.details = details


class NetworkError(ContinuaError):
    """Raised when a network request fails after all retry attempts.

    Attributes:
        retry_count: Number of retry attempts made
        cause: The original exception that caused the failure
    """

    def __init__(
        self,
        message: str = "Network request failed",
        retry_count: int = 0,
        cause: Exception | None = None,
    ) -> None:
        full_message = f"{message} (after {retry_count} retries)"
        super().__init__(full_message)
        self.message = message
        self.retry_count = retry_count
        self.__cause__ = cause

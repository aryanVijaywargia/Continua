"""Shared assertions for Python SDK tests."""

from __future__ import annotations

import re
from datetime import datetime
from typing import Any

EVENT_TS_PATTERN = re.compile(
    r"\d{4}-\d{2}-\d{2}T\d{2}:\d{2}:\d{2}\.\d{6}Z"
)
RFC3339_TIMESTAMP_PATTERN = re.compile(
    r"^(?P<head>\d{4}-\d{2}-\d{2}T\d{2}:\d{2}:\d{2})"
    r"(?:\.(?P<fraction>\d+))?"
    r"(?P<offset>Z|[+-]\d{2}:\d{2})$"
)


def assert_event_metadata(event: dict[str, Any], *, sequence: int) -> None:
    """Assert common explicit-event metadata fields emitted by the SDK."""
    assert event.get("sequence") == sequence
    event_ts = event.get("event_ts")
    assert isinstance(event_ts, str)
    assert EVENT_TS_PATTERN.fullmatch(event_ts) is not None


def parse_rfc3339_timestamp(value: str) -> datetime:
    """Parse RFC 3339 timestamps with either Z or explicit UTC offsets."""
    match = RFC3339_TIMESTAMP_PATTERN.fullmatch(value)
    if match is None:
        raise ValueError(f"Invalid RFC 3339 timestamp: {value}")

    fraction = match.group("fraction")
    normalized_fraction = ""
    if fraction is not None:
        normalized_fraction = f".{fraction[:6].ljust(6, '0')}"

    offset = match.group("offset")
    normalized_offset = "+00:00" if offset == "Z" else offset

    return datetime.fromisoformat(
        f"{match.group('head')}{normalized_fraction}{normalized_offset}"
    )

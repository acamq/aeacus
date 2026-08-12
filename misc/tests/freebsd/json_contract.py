#!/usr/bin/env python3
# /// script
# requires-python = ">=3.12"
# dependencies = []
# ///
# ─── How to run ───
# uv run misc/tests/freebsd/json_contract.py INPUT.json

from __future__ import annotations

import json
import re
from dataclasses import dataclass
from pathlib import Path
from typing import Final

type JsonValue = None | bool | int | str | list[JsonValue] | dict[str, JsonValue]
type JsonObject = dict[str, JsonValue]

HEX40: Final = re.compile(r"^[0-9a-f]{40}$")
HEX64: Final = re.compile(r"^[0-9a-f]{64}$")


@dataclass(frozen=True, slots=True)
class ContractError(Exception):
    detail: str

    def __str__(self) -> str:
        return self.detail


def reject_constant(value: str) -> None:
    raise ContractError(detail=f"non-integer JSON number: {value}")


def unique_object(pairs: list[tuple[str, JsonValue]]) -> JsonObject:
    result: JsonObject = {}
    for key, value in pairs:
        if key in result:
            raise ContractError(detail=f"duplicate JSON key: {key}")
        result[key] = value
    return result


def load_document(path: Path) -> JsonObject:
    raw = path.read_bytes()
    if raw.startswith(b"\xef\xbb\xbf") or not raw.endswith(b"\n"):
        raise ContractError(detail=f"non-canonical framing: {path}")
    try:
        value = json.loads(
            raw,
            object_pairs_hook=unique_object,
            parse_float=reject_constant,
            parse_constant=reject_constant,
        )
    except (json.JSONDecodeError, UnicodeDecodeError) as error:
        raise ContractError(detail=f"invalid JSON {path}: {error}") from error
    if not isinstance(value, dict):
        raise ContractError(detail=f"top-level JSON must be an object: {path}")
    reject_null(value, "$")
    return value


def reject_null(value: JsonValue, path: str) -> None:
    if value is None:
        raise ContractError(detail=f"null forbidden at {path}")
    if isinstance(value, list):
        for index, item in enumerate(value):
            reject_null(item, f"{path}[{index}]")
    elif isinstance(value, dict):
        for key, item in value.items():
            reject_null(item, f"{path}.{key}")


def require_keys(value: JsonObject, expected: set[str], path: str) -> None:
    actual = set(value)
    if actual != expected:
        raise ContractError(detail=f"wrong keys at {path}: {sorted(actual ^ expected)}")


def require_string(value: JsonValue, path: str) -> str:
    if not isinstance(value, str):
        raise ContractError(detail=f"expected string at {path}")
    return value


def require_int(value: JsonValue, path: str, *, positive: bool = False) -> int:
    if isinstance(value, bool) or not isinstance(value, int):
        raise ContractError(detail=f"expected integer at {path}")
    if value < (1 if positive else 0):
        raise ContractError(detail=f"integer out of range at {path}")
    return value


def require_bool(value: JsonValue, path: str) -> bool:
    if not isinstance(value, bool):
        raise ContractError(detail=f"expected boolean at {path}")
    return value


def require_object(value: JsonValue, path: str) -> JsonObject:
    if not isinstance(value, dict):
        raise ContractError(detail=f"expected object at {path}")
    return value


def require_array(value: JsonValue, path: str) -> list[JsonValue]:
    if not isinstance(value, list):
        raise ContractError(detail=f"expected array at {path}")
    return value


def require_hash(value: JsonValue, path: str, width: int = 64) -> str:
    text = require_string(value, path)
    pattern = HEX64 if width == 64 else HEX40
    if pattern.fullmatch(text) is None:
        raise ContractError(detail=f"invalid hash at {path}")
    return text


def canonical_bytes(value: JsonObject) -> bytes:
    return (json.dumps(value, ensure_ascii=False, sort_keys=True, separators=(",", ":")) + "\n").encode()

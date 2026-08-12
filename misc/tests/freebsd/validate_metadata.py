#!/usr/bin/env python3
# /// script
# requires-python = ">=3.12"
# dependencies = []
# ///
# ─── How to run ───
# uv run misc/tests/freebsd/validate.py metadata INPUT.json

from __future__ import annotations

from pathlib import Path
from typing import Final

from misc.tests.freebsd.json_contract import ContractError, JsonObject, require_array, require_bool, require_hash, require_int, require_keys, require_object, require_string
from misc.tests.freebsd.validate_abi import ACTION_SHA, EXPECTED_ROWS

TOP_KEYS: Final = {"schema", "probe_source_sha256", "action_sha", "workflow_commit", "run_id", "run_attempt", "rows"}
ROW_KEYS: Final = {"release", "arch", "image_sha256", "paths"}
ABSENT_KEYS: Final = {"path", "present"}
PRESENT_KEYS: Final = {"path", "present", "type", "uid", "gid", "mode", "nlink", "bytes"}
REGULAR_KEYS: Final = PRESENT_KEYS | {"sha256"}


def expected_paths() -> tuple[str, ...]:
    path = Path(__file__).with_name("metadata-paths.txt")
    return tuple(path.read_text(encoding="utf-8").splitlines())


def validate_path(value: JsonObject) -> str:
    present = require_bool(value.get("present"), "$.rows[].paths[].present")
    path = require_string(value.get("path"), "$.rows[].paths[].path")
    if not present:
        require_keys(value, ABSENT_KEYS, "$.rows[].paths[]")
        return path
    kind = require_string(value.get("type"), "$.rows[].paths[].type")
    require_keys(value, REGULAR_KEYS if kind == "Regular File" else PRESENT_KEYS, "$.rows[].paths[]")
    for field in ("uid", "gid", "nlink", "bytes"):
        require_int(value[field], f"$.rows[].paths[].{field}")
    require_string(value["mode"], "$.rows[].paths[].mode")
    if kind == "Regular File": require_hash(value["sha256"], "$.rows[].paths[].sha256")
    return path


def validate_metadata_document(document: JsonObject) -> None:
    require_keys(document, TOP_KEYS, "$")
    if require_string(document["schema"], "$.schema") != "aeacus-freebsd-base-metadata-v1": raise ContractError(detail="wrong metadata schema")
    require_hash(document["probe_source_sha256"], "$.probe_source_sha256")
    if require_string(document["action_sha"], "$.action_sha") != ACTION_SHA: raise ContractError(detail="wrong action SHA")
    require_hash(document["workflow_commit"], "$.workflow_commit", 40)
    require_int(document["run_id"], "$.run_id", positive=True)
    require_int(document["run_attempt"], "$.run_attempt", positive=True)
    row_keys: list[tuple[str, str]] = []
    for item in require_array(document["rows"], "$.rows"):
        row = require_object(item, "$.rows[]")
        require_keys(row, ROW_KEYS, "$.rows[]")
        row_keys.append((require_string(row["release"], "$.rows[].release"), require_string(row["arch"], "$.rows[].arch")))
        require_hash(row["image_sha256"], "$.rows[].image_sha256")
        paths = tuple(validate_path(require_object(value, "$.rows[].paths[]")) for value in require_array(row["paths"], "$.rows[].paths"))
        if paths != expected_paths(): raise ContractError(detail="wrong metadata path inventory/order")
    if tuple(row_keys) != EXPECTED_ROWS: raise ContractError(detail="wrong metadata rows/order")

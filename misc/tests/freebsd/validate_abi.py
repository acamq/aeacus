#!/usr/bin/env python3
# /// script
# requires-python = ">=3.12"
# dependencies = []
# ///
# ─── How to run ───
# uv run misc/tests/freebsd/validate.py abi INPUT.json

from __future__ import annotations

from typing import Final

from misc.tests.freebsd.json_contract import (
    ContractError,
    JsonObject,
    require_array,
    require_bool,
    require_hash,
    require_int,
    require_keys,
    require_object,
    require_string,
)

ACTION_SHA: Final = "5ea7e8e4677bd726033a10b094ba1c5762b15dee"
TOP_KEYS: Final = {"schema", "probe_source_sha256", "action_sha", "workflow_commit", "run_id", "run_attempt", "rows"}
ROW_KEYS: Final = {"release", "arch", "byte_order", "image_url", "image_bytes", "image_sha256", "freebsd_version", "user_h_sha256", "proc_h_sha256", "p_traced", "struct_size", "fields"}
FIELD_KEYS: Final = {"name", "offset", "width", "signed"}
FIELD_NAMES: Final = ("ki_structsize", "ki_pid", "ki_flag", "ki_tracer", "ki_start")
EXPECTED_ROWS: Final = (("14.4", "arm64"), ("14.4", "x86-64"), ("15.1", "arm64"), ("15.1", "x86-64"))


def validate_field(value: JsonObject, index: int) -> tuple[str, int, int, bool]:
    path = f"$.rows[].fields[{index}]"
    require_keys(value, FIELD_KEYS, path)
    name = require_string(value["name"], f"{path}.name")
    offset = require_int(value["offset"], f"{path}.offset")
    width = require_int(value["width"], f"{path}.width", positive=True)
    signed = require_bool(value["signed"], f"{path}.signed")
    return name, offset, width, signed


def validate_row(value: JsonObject) -> tuple[str, str]:
    require_keys(value, ROW_KEYS, "$.rows[]")
    release = require_string(value["release"], "$.rows[].release")
    arch = require_string(value["arch"], "$.rows[].arch")
    if require_string(value["byte_order"], "$.rows[].byte_order") != "little":
        raise ContractError(detail="unsupported byte order")
    url = require_string(value["image_url"], "$.rows[].image_url")
    if not url.startswith("https://github.com/cross-platform-actions/freebsd-builder/releases/download/v0.15.0/"):
        raise ContractError(detail="unlocked image URL")
    require_int(value["image_bytes"], "$.rows[].image_bytes", positive=True)
    for name in ("image_sha256", "user_h_sha256", "proc_h_sha256"):
        require_hash(value[name], f"$.rows[].{name}")
    require_string(value["freebsd_version"], "$.rows[].freebsd_version")
    if require_int(value["p_traced"], "$.rows[].p_traced") != 0x800:
        raise ContractError(detail="P_TRACED mismatch")
    if require_int(value["struct_size"], "$.rows[].struct_size") != 1088:
        raise ContractError(detail="kinfo_proc size mismatch")
    fields = [validate_field(require_object(item, "$.rows[].fields[]"), index) for index, item in enumerate(require_array(value["fields"], "$.rows[].fields"))]
    if tuple(field[0] for field in fields) != FIELD_NAMES:
        raise ContractError(detail="wrong kinfo field inventory/order")
    if fields[0][1:3] != (0, 4):
        raise ContractError(detail="ki_structsize invariant mismatch")
    if fields[4][2:] != (16, False):
        raise ContractError(detail="ki_start width/signedness mismatch")
    if any(offset + width > 1088 for _, offset, width, _ in fields):
        raise ContractError(detail="kinfo field out of bounds")
    return release, arch


def validate_abi_document(document: JsonObject) -> None:
    require_keys(document, TOP_KEYS, "$")
    if require_string(document["schema"], "$.schema") != "aeacus-freebsd-kinfo-v1":
        raise ContractError(detail="wrong ABI schema")
    require_hash(document["probe_source_sha256"], "$.probe_source_sha256")
    if require_string(document["action_sha"], "$.action_sha") != ACTION_SHA:
        raise ContractError(detail="wrong action SHA")
    require_hash(document["workflow_commit"], "$.workflow_commit", 40)
    require_int(document["run_id"], "$.run_id", positive=True)
    require_int(document["run_attempt"], "$.run_attempt", positive=True)
    rows = [validate_row(require_object(item, "$.rows[]")) for item in require_array(document["rows"], "$.rows")]
    if tuple(rows) != EXPECTED_ROWS:
        raise ContractError(detail="wrong ABI rows/order")

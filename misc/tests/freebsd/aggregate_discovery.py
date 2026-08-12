#!/usr/bin/env python3
# /// script
# requires-python = ">=3.12"
# dependencies = []
# ///
# ─── How to run ───
# uv run misc/tests/freebsd/aggregate_discovery.py INPUT_ROOT OUTPUT_ROOT

from __future__ import annotations

import os
import sys
from pathlib import Path
from typing import Final

from misc.tests.freebsd.json_contract import JsonObject, JsonValue, canonical_bytes
from misc.tests.freebsd.validate_abi import ACTION_SHA

FIELD_ORDER: Final = ("ki_structsize", "ki_pid", "ki_flag", "ki_tracer", "ki_start")


def parse_probe(path: Path) -> tuple[int, int, list[JsonValue]]:
    values: dict[str, tuple[int, int, bool]] = {}
    struct_size = p_traced = -1
    for line in path.read_text(encoding="ascii").splitlines():
        parts = line.split("\t")
        match parts:
            case ["struct_size", size]: struct_size = int(size)
            case ["p_traced", traced]: p_traced = int(traced)
            case [name, offset, width, signed]: values[name] = (int(offset), int(width), signed == "1")
            case _: raise RuntimeError(f"malformed probe row: {line}")
    fields: list[JsonValue] = [{"name": name, "offset": values[name][0], "width": values[name][1], "signed": values[name][2]} for name in FIELD_ORDER]
    return struct_size, p_traced, fields


def parse_metadata(path: Path) -> list[JsonValue]:
    lines = path.read_text(encoding="utf-8").splitlines()
    rows: list[JsonValue] = []
    index = 0
    while index < len(lines):
        parts = lines[index].split("\t")
        if len(parts) == 2 and parts[1] == "absent":
            rows.append({"path": parts[0], "present": False})
            index += 1
            continue
        if len(parts) != 7 or index + 1 >= len(lines):
            raise RuntimeError(f"malformed metadata at line {index + 1}")
        file_hash = lines[index + 1]
        row: JsonObject = {"path": parts[0], "present": True, "type": parts[1], "uid": int(parts[2]), "gid": int(parts[3]), "mode": parts[4], "nlink": int(parts[5]), "bytes": int(parts[6])}
        if file_hash != "-": row["sha256"] = file_hash
        rows.append(row)
        index += 2
    return sorted(rows, key=lambda item: str(item["path"]) if isinstance(item, dict) else "")


def lock_rows(path: Path) -> dict[tuple[str, str], tuple[str, int, str]]:
    result: dict[tuple[str, str], tuple[str, int, str]] = {}
    for line in path.read_text(encoding="utf-8").splitlines()[2:]:
        release, arch, _, url, size, digest = line.split("\t")
        result[(release, arch)] = (url, int(size), digest)
    return result


def main(arguments: list[str]) -> int:
    source, output = map(Path, arguments)
    workflow_commit = os.environ["GITHUB_SHA"]
    run_id = int(os.environ["GITHUB_RUN_ID"])
    attempt = int(os.environ["GITHUB_RUN_ATTEMPT"])
    if len(workflow_commit) != 40 or any(character not in "0123456789abcdef" for character in workflow_commit): raise RuntimeError("invalid workflow commit")
    if run_id < 1 or attempt < 1: raise RuntimeError("invalid run identity")
    locks = lock_rows(Path(".github/freebsd-images.lock"))
    abi_rows: list[JsonValue] = []
    metadata_rows: list[JsonValue] = []
    probe_hash = ""
    for release, arch in sorted(locks):
        row_dir = source / release / arch
        struct_size, p_traced, fields = parse_probe(row_dir / "probe.tsv")
        current_probe_hash = (row_dir / "probe-source.sha256").read_text(encoding="ascii").strip()
        probe_hash = probe_hash or current_probe_hash
        if current_probe_hash != probe_hash: raise RuntimeError("probe source mismatch")
        url, image_bytes, image_hash = locks[(release, arch)]
        abi_rows.append({"release": release, "arch": arch, "byte_order": "little", "image_url": url, "image_bytes": image_bytes, "image_sha256": image_hash, "freebsd_version": (row_dir / "freebsd-version.txt").read_text(encoding="ascii").strip(), "user_h_sha256": (row_dir / "user-h.sha256").read_text(encoding="ascii").strip(), "proc_h_sha256": (row_dir / "proc-h.sha256").read_text(encoding="ascii").strip(), "p_traced": p_traced, "struct_size": struct_size, "fields": fields})
        metadata_rows.append({"release": release, "arch": arch, "image_sha256": image_hash, "paths": parse_metadata(row_dir / "metadata.tsv")})
    common: JsonObject = {"probe_source_sha256": probe_hash, "action_sha": ACTION_SHA, "workflow_commit": workflow_commit, "run_id": run_id, "run_attempt": attempt}
    abi: JsonObject = {"schema": "aeacus-freebsd-kinfo-v1", **common, "rows": abi_rows}
    metadata: JsonObject = {"schema": "aeacus-freebsd-base-metadata-v1", **common, "rows": metadata_rows}
    output.mkdir(parents=True, exist_ok=False)
    (output / "kinfo-layouts.json").write_bytes(canonical_bytes(abi))
    (output / "base-metadata.json").write_bytes(canonical_bytes(metadata))
    return 0


if __name__ == "__main__":
    raise SystemExit(main(sys.argv[1:]))

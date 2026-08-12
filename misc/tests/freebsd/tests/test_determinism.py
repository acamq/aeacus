#!/usr/bin/env python3
# /// script
# requires-python = ">=3.12"
# dependencies = []
# ///
# ─── How to run ───
# uv run misc/tests/freebsd/tests/test_determinism.py

from __future__ import annotations

import importlib.util
import os
import sys
import tempfile
from pathlib import Path
from typing import Final

ROOT: Final = Path(__file__).resolve().parents[4]
FREEBSD: Final = ROOT / "misc/tests/freebsd"
sys.path.insert(0, str(ROOT))

from misc.tests.freebsd.tests.test_contracts import valid_abi


def load_aggregator() -> object:
    path = FREEBSD / "aggregate_discovery.py"
    spec = importlib.util.spec_from_file_location("aggregate_discovery", path)
    assert spec is not None and spec.loader is not None
    module = importlib.util.module_from_spec(spec)
    sys.modules[spec.name] = module
    spec.loader.exec_module(module)
    return module


def write_rows(root: Path, document: dict[str, object]) -> None:
    for row_value in document["rows"]:
        assert isinstance(row_value, dict)
        row = root / str(row_value["release"]) / str(row_value["arch"])
        row.mkdir(parents=True)
        probe = [f"struct_size\t{row_value['struct_size']}", f"p_traced\t{row_value['p_traced']}"]
        for field_value in row_value["fields"]:
            assert isinstance(field_value, dict)
            probe.append(f"{field_value['name']}\t{field_value['offset']}\t{field_value['width']}\t{1 if field_value['signed'] else 0}")
        (row / "probe.tsv").write_text("\n".join(probe) + "\n", encoding="ascii")
        (row / "freebsd-version.txt").write_text(str(row_value["freebsd_version"]) + "\n", encoding="ascii")
        (row / "user-h.sha256").write_text(str(row_value["user_h_sha256"]) + "\n", encoding="ascii")
        (row / "proc-h.sha256").write_text(str(row_value["proc_h_sha256"]) + "\n", encoding="ascii")
        (row / "probe-source.sha256").write_text(str(document["probe_source_sha256"]) + "\n", encoding="ascii")
        paths = (FREEBSD / "metadata-paths.txt").read_text(encoding="utf-8").splitlines()
        metadata = []
        for path in paths:
            if path == "/etc/master.passwd": metadata.append(f"{path}\tRegular File\t0\t0\t0600\t1\t1\nstat-only\n")
            else: metadata.append(f"{path}\tabsent\n")
        (row / "metadata.tsv").write_text("".join(metadata), encoding="utf-8")


def main() -> int:
    document = valid_abi()
    os.environ.update(GITHUB_SHA=str(document["workflow_commit"]), GITHUB_RUN_ID="1", GITHUB_RUN_ATTEMPT="1")
    module = load_aggregator()
    with tempfile.TemporaryDirectory() as directory:
        root = Path(directory)
        source = root / "input"
        write_rows(source, document)
        outputs: list[bytes] = []
        for name in ("first", "second"):
            assert module.main([str(source), str(root / name)]) == 0
            outputs.append((root / name / "kinfo-layouts.json").read_bytes() + (root / name / "base-metadata.json").read_bytes())
        assert outputs[0] == outputs[1]
    print("Task 10 deterministic discovery aggregation: PASS")
    return 0


if __name__ == "__main__":
    raise SystemExit(main())

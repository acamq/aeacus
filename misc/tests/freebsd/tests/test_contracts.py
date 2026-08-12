#!/usr/bin/env python3
# /// script
# requires-python = ">=3.12"
# dependencies = []
# ///
# ─── How to run ───
# uv run misc/tests/freebsd/tests/test_contracts.py

from __future__ import annotations

import importlib.util
import json
import os
import re
import subprocess
import sys
import tempfile
from pathlib import Path
from types import ModuleType
from typing import Final

ROOT: Final = Path(__file__).resolve().parents[4]
FREEBSD: Final = ROOT / "misc/tests/freebsd"
PRODUCTS: Final = (
    ROOT / ".github/freebsd-images.lock",
    ROOT / ".github/workflows/freebsd-abi-discovery.yml",
    ROOT / ".github/workflows/freebsd-fixture-build.yml",
    FREEBSD / "validate.py",
    FREEBSD / "serve-image.py",
    FREEBSD / "kinfo_probe.c",
)
ACTION_SHAS: Final = {
    "actions/checkout": "11bd71901bbe5b1630ceea73d27597364c9af683",
    "actions/upload-artifact": "ea165f8d65b6e75b540449e92b4886f43607fa02",
    "actions/download-artifact": "d3f86a106a0bac45b974a628896c90dbdf5c8093",
    "cross-platform-actions/action": "5ea7e8e4677bd726033a10b094ba1c5762b15dee",
}


def require_products() -> None:
    missing = [str(path.relative_to(ROOT)) for path in PRODUCTS if not path.exists()]
    assert not missing, f"missing Task 10 products: {missing}"


def load_validator() -> ModuleType:
    path = FREEBSD / "validate.py"
    sys.path.insert(0, str(ROOT))
    spec = importlib.util.spec_from_file_location("freebsd_validate", path)
    assert spec is not None and spec.loader is not None
    module = importlib.util.module_from_spec(spec)
    sys.modules[spec.name] = module
    spec.loader.exec_module(module)
    return module


def valid_abi() -> dict[str, object]:
    fields = [
        {"name": "ki_structsize", "offset": 0, "width": 4, "signed": False},
        {"name": "ki_pid", "offset": 72, "width": 4, "signed": True},
        {"name": "ki_flag", "offset": 368, "width": 8, "signed": True},
        {"name": "ki_tracer", "offset": 0, "width": 4, "signed": True},
        {"name": "ki_start", "offset": 0, "width": 16, "signed": False},
    ]
    rows: list[dict[str, object]] = []
    locks = {
        ("14.4", "arm64"): (345243648, "78fa20cab5254755357f2ee71196138db23ce1a95674c9f34beed44dee1391a6"),
        ("14.4", "x86-64"): (377154048, "a124dfec63f9a00b6f90f13d96a40f124b33bd8a92f38391efd33270bd7000c3"),
        ("15.1", "arm64"): (358350848, "5dee8bedce8b1b3a53776f8f60d875b0dd0d696e1258ffcf696885b0dec39118"),
        ("15.1", "x86-64"): (390723584, "d7045aa82b38f43af0ab778c6a89725703f5223d04a1d7c3b52bf66b88f164fb"),
    }
    for release, arch in sorted(locks):
        size, image_hash = locks[(release, arch)]
        rows.append({
            "release": release, "arch": arch, "byte_order": "little",
            "image_url": f"https://github.com/cross-platform-actions/freebsd-builder/releases/download/v0.15.0/freebsd-{release}-{arch}.qcow2",
            "image_bytes": size, "image_sha256": image_hash,
            "freebsd_version": f"{release}-RELEASE", "user_h_sha256": "b" * 64,
            "proc_h_sha256": "c" * 64, "p_traced": 2048,
            "struct_size": 1088, "fields": fields,
        })
    return {
        "schema": "aeacus-freebsd-kinfo-v1", "probe_source_sha256": "d" * 64,
        "action_sha": "5ea7e8e4677bd726033a10b094ba1c5762b15dee",
        "workflow_commit": "e" * 40, "run_id": 1, "run_attempt": 1, "rows": rows,
    }


def assert_rejected(module: ModuleType, document: dict[str, object]) -> None:
    with tempfile.TemporaryDirectory() as directory:
        path = Path(directory) / "input.json"
        path.write_text(json.dumps(document), encoding="utf-8")
        result = subprocess.run(
            [sys.executable, str(FREEBSD / "validate.py"), "abi", str(path)],
            cwd=ROOT, check=False, capture_output=True, text=True,
        )
        assert result.returncode != 0, result.stdout


def test_corruptions(module: ModuleType) -> None:
    document = valid_abi()
    module.validate_abi_document(document)
    for mutation in ("schema", "unknown", "null", "duplicate", "head"):
        corrupt = json.loads(json.dumps(document))
        if mutation == "schema":
            corrupt["schema"] = "wrong"
        elif mutation == "unknown":
            corrupt["unknown"] = True
        elif mutation == "null":
            corrupt["rows"][0]["freebsd_version"] = None
        elif mutation == "duplicate":
            corrupt["rows"].append(corrupt["rows"][0])
        else:
            corrupt["workflow_commit"] = "f" * 39
        assert_rejected(module, corrupt)


def test_executables() -> None:
    scripts = [path for path in FREEBSD.rglob("*") if path.suffix in {".py", ".sh"} and path.name != "__init__.py"]
    for path in scripts:
        assert os.access(path, os.X_OK), f"not executable: {path.relative_to(ROOT)}"


def test_workflows() -> None:
    expected = {
        "freebsd-abi-discovery.yml": ("freebsd-abi-discovery", "abi-${{ github.sha }}-${{ github.run_attempt }}", "freebsd-abi-${{ github.sha }}-${{ github.run_attempt }}"),
        "freebsd-fixture-build.yml": ("freebsd-fixture-build", "fixtures-${{ github.sha }}-${{ github.run_attempt }}", "fixture-build-${{ github.sha }}-${{ github.run_attempt }}"),
    }
    for filename, (name, run_name, artifact) in expected.items():
        text = (ROOT / ".github/workflows" / filename).read_text(encoding="utf-8")
        assert f"name: {name}\n" in text
        assert f"run-name: {run_name}\n" in text
        assert "branches:\n      - integrate/freebsd-support" in text
        assert "pull_request:" not in text and "workflow_dispatch:" not in text
        assert "permissions:\n  contents: read" in text
        assert artifact in text
        for size, digest in (("345243648", "78fa20cab5254755357f2ee71196138db23ce1a95674c9f34beed44dee1391a6"), ("377154048", "a124dfec63f9a00b6f90f13d96a40f124b33bd8a92f38391efd33270bd7000c3"), ("358350848", "5dee8bedce8b1b3a53776f8f60d875b0dd0d696e1258ffcf696885b0dec39118"), ("390723584", "d7045aa82b38f43af0ab778c6a89725703f5223d04a1d7c3b52bf66b88f164fb")):
            assert f"image_bytes: '{size}'" in text and f"image_sha256: {digest}" in text
        assert "cache" not in text and "secrets." not in text
        assert not re.search(r"\b(gh release|git tag|releases/(assets|upload)|contents: write)\b", text)
        uses = re.findall(r"uses:\s*([^@\s]+)@([0-9a-f]+)", text)
        assert uses
        for action, sha in uses:
            assert ACTION_SHAS.get(action) == sha, (action, sha)


def test_image_lock(module: ModuleType) -> None:
    module.validate_image_lock(ROOT / ".github/freebsd-images.lock")


def main() -> int:
    require_products()
    module = load_validator()
    test_corruptions(module)
    test_image_lock(module)
    test_workflows()
    test_executables()
    print("Task 10 portable contracts: PASS")
    return 0


if __name__ == "__main__":
    raise SystemExit(main())

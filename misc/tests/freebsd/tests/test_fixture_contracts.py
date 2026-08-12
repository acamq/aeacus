#!/usr/bin/env python3
# /// script
# requires-python = ">=3.12"
# dependencies = []
# ///
# ─── How to run ───
# uv run misc/tests/freebsd/tests/test_fixture_contracts.py

from __future__ import annotations

import copy
import hashlib
import sys
import tempfile
from pathlib import Path
from typing import Final

ROOT: Final = Path(__file__).resolve().parents[4]
sys.path.insert(0, str(ROOT))

from misc.tests.freebsd.json_contract import ContractError, canonical_bytes
from misc.tests.freebsd.validate_fixture import validate_fixture

SHA: Final = "a" * 64


def documents(root: Path) -> tuple[dict[str, object], dict[str, object]]:
    roots = [{"release": release, "arch": arch, "name": name, "source": source} for release in ("14.4", "15.1") for arch in ("arm64", "x86-64") for name, source in (("xfce", "ports"), ("lightdm", "ports"), ("lightdm-gtk-greeter", "ports"), ("xdg-utils", "ports"))]
    roots.extend({"release": "15.1", "arch": arch, "name": "FreeBSD-runtime", "source": "pkgbase"} for arch in ("arm64", "x86-64"))
    packages: list[dict[str, object]] = []
    for row in roots:
        release, arch, name, source = (str(row[key]) for key in ("release", "arch", "name", "source"))
        directory = root / release / arch / source
        package = directory / "packages" / f"{name}.pkg"
        compact = directory / "compact-manifests" / f"{name}.pkg.compact-manifest"
        package.parent.mkdir(parents=True, exist_ok=True)
        compact.parent.mkdir(parents=True, exist_ok=True)
        package.write_bytes(name.encode())
        compact.write_text(f'{{"name":"{name}","origin":"test/{name}","version":"1"}}', encoding="utf-8")
        packages.append({"release": release, "arch": arch, "name": name, "version": "1", "origin": f"test/{name}", "source": source, "source_archive_sha256": SHA, "package_sha256": hashlib.sha256(package.read_bytes()).hexdigest(), "compact_manifest_sha256": hashlib.sha256(compact.read_bytes()).hexdigest(), "dependencies": [], "path": str(package.relative_to(root)), "compact_manifest_path": str(compact.relative_to(root)), "bytes": package.stat().st_size})
    sources = root / "sources"
    sources.mkdir()
    (sources / "ports.tar").write_bytes(b"ports")
    (sources / "pkgbase.tar").write_bytes(b"pkgbase")
    catalog = {"schema": "aeacus-freebsd-source-catalog-v1", "ports_commit": "06361922e0ed9680cc3e302d7615901b56afd90f", "pkgbase_commit": "aadd58dddcbc78f4d5594827b46b5633552b15ce", "ports_archive_sha256": hashlib.sha256(b"ports").hexdigest(), "pkgbase_archive_sha256": hashlib.sha256(b"pkgbase").hexdigest(), "roots": sorted(roots, key=lambda row: tuple(str(row[key]) for key in ("release", "arch", "name", "source"))), "packages": sorted(packages, key=lambda row: tuple(str(row[key]) for key in ("release", "arch", "origin", "name", "version")))}
    (root / "source-catalog.json").write_bytes(canonical_bytes(catalog))
    manifest = {"schema": "aeacus-freebsd-fixture-build-v1", "head_sha": "b" * 40, "source_date_epoch": 1, "builder_images": [], "source_catalog_sha256": hashlib.sha256((root / "source-catalog.json").read_bytes()).hexdigest(), "assets": [{} for _ in packages]}
    return catalog, manifest


def rejected(root: Path, catalog: dict[str, object], manifest: dict[str, object]) -> None:
    try:
        validate_fixture(root, catalog, manifest)
    except ContractError:
        return
    raise AssertionError("fixture corruption accepted")


def main() -> int:
    with tempfile.TemporaryDirectory() as directory:
        root = Path(directory)
        catalog, manifest = documents(root)
        validate_fixture(root, catalog, manifest)
        for mutation in ("schema", "missing-root", "extra-package", "hash", "null"):
            damaged = copy.deepcopy(catalog)
            if mutation == "schema": damaged["schema"] = "wrong"
            elif mutation == "missing-root": damaged["roots"].pop()
            elif mutation == "extra-package": damaged["packages"].append(copy.deepcopy(damaged["packages"][0]))
            elif mutation == "hash": damaged["ports_archive_sha256"] = "0" * 64
            else: damaged["packages"][0]["origin"] = None
            rejected(root, damaged, manifest)
    print("Task 10 fixture corruption contracts: PASS")
    return 0


if __name__ == "__main__":
    raise SystemExit(main())

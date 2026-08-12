#!/usr/bin/env python3
# /// script
# requires-python = ">=3.12"
# dependencies = []
# ///
# ─── How to run ───
# uv run misc/tests/freebsd/aggregate_fixtures.py INPUT_ROOT OUTPUT_ROOT

from __future__ import annotations

import hashlib
import os
import sys
from pathlib import Path
from typing import Final

from misc.tests.freebsd.json_contract import JsonObject, JsonValue, canonical_bytes, load_document
from misc.tests.freebsd.package_select import main as select_packages

PORTS_COMMIT: Final = "06361922e0ed9680cc3e302d7615901b56afd90f"
PKGBASE_COMMIT: Final = "aadd58dddcbc78f4d5594827b46b5633552b15ce"
IMAGE_HASHES: Final = {
    ("14.4", "arm64"): "78fa20cab5254755357f2ee71196138db23ce1a95674c9f34beed44dee1391a6",
    ("14.4", "x86-64"): "a124dfec63f9a00b6f90f13d96a40f124b33bd8a92f38391efd33270bd7000c3",
    ("15.1", "arm64"): "5dee8bedce8b1b3a53776f8f60d875b0dd0d696e1258ffcf696885b0dec39118",
    ("15.1", "x86-64"): "d7045aa82b38f43af0ab778c6a89725703f5223d04a1d7c3b52bf66b88f164fb",
}


def sha256(path: Path) -> str:
    return hashlib.sha256(path.read_bytes()).hexdigest()


def package_key(row: JsonObject) -> tuple[str, str, str, str, str]:
    return str(row["release"]), str(row["arch"]), str(row["origin"]), str(row["name"]), str(row["version"])


def add_package_assets(output: Path, packages: list[JsonObject]) -> list[JsonValue]:
    assets: list[JsonValue] = []
    for package in packages:
        release = str(package["release"])
        arch = str(package["arch"])
        kind = str(package["source"])
        matches = list((output / release / arch / kind / "packages").glob("*.pkg"))
        package_path = next((path for path in matches if sha256(path) == package["package_sha256"]), None)
        if package_path is None:
            raise RuntimeError(f"package bytes absent: {release}/{arch}/{package['name']}")
        manifest = output / release / arch / kind / "compact-manifests" / f"{package_path.name}.compact-manifest"
        if not manifest.is_file():
            raise RuntimeError(f"compact manifest absent: {manifest}")
        commit = PORTS_COMMIT if kind == "ports" else PKGBASE_COMMIT
        package["path"] = str(package_path.relative_to(output))
        package["compact_manifest_path"] = str(manifest.relative_to(output))
        assets.append({"release": release, "arch": arch, "origin": package["origin"], "name": package["name"], "version": package["version"], "filename": package_path.name, "bytes": package_path.stat().st_size, "sha256": package["package_sha256"], "package_manifest_sha256": sha256(manifest), "verification_kind": f"{kind}-source-build", "verification_fingerprint": f"git:{commit}"})
    return sorted(assets, key=lambda value: tuple(str(value[key]) for key in ("release", "arch", "origin", "name", "version", "filename")) if isinstance(value, dict) else ())


def assert_exact_closure(roots: list[JsonValue], packages: list[JsonObject]) -> None:
    package_keys = {(str(value["release"]), str(value["arch"]), str(value["name"])) for value in packages}
    reachable: set[tuple[str, str, str]] = set()
    pending = [(str(value["release"]), str(value["arch"]), str(value["name"])) for value in roots if isinstance(value, dict)]
    by_key = {(str(value["release"]), str(value["arch"]), str(value["name"])): value for value in packages}
    while pending:
        key = pending.pop()
        if key in reachable: continue
        package = by_key.get(key)
        if package is None: raise RuntimeError(f"missing root/dependency package: {key}")
        reachable.add(key)
        dependencies = package["dependencies"]
        if not isinstance(dependencies, list): raise RuntimeError(f"invalid dependency list: {key}")
        for dependency in dependencies:
            if not isinstance(dependency, dict): raise RuntimeError(f"invalid dependency: {key}")
            pending.append((key[0], key[1], str(dependency["name"])))
    if reachable != package_keys: raise RuntimeError(f"extra unreachable package rows: {sorted(package_keys - reachable)}")


def main(arguments: list[str]) -> int:
    source, output = map(Path, arguments)
    output.mkdir(parents=True, exist_ok=False)
    source_output = output / "sources"
    source_output.mkdir()
    roots: list[JsonValue] = []
    packages: list[JsonObject] = []
    source_hashes: dict[str, str] = {}
    for release, arch in sorted(IMAGE_HASHES):
        row = source / release / arch
        for kind, names in (("ports", ("xfce", "lightdm", "lightdm-gtk-greeter", "xdg-utils")), ("pkgbase", ("FreeBSD-runtime",))):
            raw = row / f"{kind}-raw"
            if not raw.exists():
                if kind == "pkgbase" and release == "14.4": continue
                raise RuntimeError(f"missing fixture packages: {raw}")
            archive = row / "sources" / ("ports.tar" if kind == "ports" else "pkgbase.tar")
            digest = sha256(archive)
            destination_archive = source_output / archive.name
            if destination_archive.exists():
                if sha256(destination_archive) != digest:
                    raise RuntimeError(f"source archive drift: {kind}")
            else:
                destination_archive.write_bytes(archive.read_bytes())
            fragment_root = output / release / arch / kind
            if select_packages([kind, release, arch, digest, ",".join(names), str(raw), str(fragment_root)]) != 0:
                raise RuntimeError(f"package selection failed: {release}/{arch}/{kind}")
            fragment_path = fragment_root / "fragment.json"
            fragment = load_document(fragment_path)
            fragment_roots = fragment["roots"]
            if not isinstance(fragment_roots, list) or tuple(str(value) for value in fragment_roots) != names:
                raise RuntimeError(f"root mismatch: {fragment_path}")
            roots.extend({"release": release, "arch": arch, "name": name, "source": kind} for name in names)
            fragment_packages = fragment["packages"]
            if not isinstance(fragment_packages, list): raise RuntimeError("invalid package list")
            for value in fragment_packages:
                if not isinstance(value, dict): raise RuntimeError("invalid package fragment")
                row_value: JsonObject = dict(value)
                filename = str(row_value.pop("filename"))
                row_value["path"] = f"{release}/{arch}/{kind}/packages/{filename}"
                row_value["compact_manifest_path"] = f"{release}/{arch}/{kind}/compact-manifests/{filename}.compact-manifest"
                package_bytes = row_value["bytes"]
                if isinstance(package_bytes, bool) or not isinstance(package_bytes, int): raise RuntimeError("invalid package byte size")
                packages.append(row_value)
            prior = source_hashes.setdefault(kind, digest)
            if prior != digest: raise RuntimeError(f"source archive drift: {kind}")
    roots.sort(key=lambda value: tuple(str(value[key]) for key in ("release", "arch", "name", "source")) if isinstance(value, dict) else ())
    packages.sort(key=package_key)
    assert_exact_closure(roots, packages)
    package_values: list[JsonValue] = [value for value in packages]
    catalog: JsonObject = {"schema": "aeacus-freebsd-source-catalog-v1", "ports_commit": PORTS_COMMIT, "pkgbase_commit": PKGBASE_COMMIT, "ports_archive_sha256": source_hashes["ports"], "pkgbase_archive_sha256": source_hashes["pkgbase"], "roots": roots, "packages": package_values}
    catalog_path = output / "source-catalog.json"
    catalog_path.write_bytes(canonical_bytes(catalog))
    assets = add_package_assets(output, packages)
    head_sha = os.environ["GITHUB_SHA"]
    epoch = int(os.environ["SOURCE_DATE_EPOCH"])
    if len(head_sha) != 40 or any(character not in "0123456789abcdef" for character in head_sha) or epoch < 0: raise RuntimeError("invalid fixture run binding")
    manifest: JsonObject = {"schema": "aeacus-freebsd-fixture-build-v1", "head_sha": head_sha, "source_date_epoch": epoch, "builder_images": [{"release": release, "arch": arch, "image_sha256": IMAGE_HASHES[(release, arch)]} for release, arch in sorted(IMAGE_HASHES)], "source_catalog_sha256": sha256(catalog_path), "assets": assets}
    (output / "fixture-build.json").write_bytes(canonical_bytes(manifest))
    return 0


if __name__ == "__main__":
    raise SystemExit(main(sys.argv[1:]))

#!/usr/bin/env python3
# /// script
# requires-python = ">=3.12"
# dependencies = []
# ///
# ─── How to run ───
# uv run misc/tests/freebsd/validate.py fixture ARTIFACT_ROOT

from __future__ import annotations

import hashlib
from pathlib import Path
from typing import Final

from misc.tests.freebsd.json_contract import ContractError, JsonObject, require_array, require_hash, require_int, require_keys, require_object
from misc.tests.freebsd.package_select import parse_compact_manifest

PORTS_COMMIT: Final = "06361922e0ed9680cc3e302d7615901b56afd90f"
PKGBASE_COMMIT: Final = "aadd58dddcbc78f4d5594827b46b5633552b15ce"
ROOT_NAMES: Final = ("xfce", "lightdm", "lightdm-gtk-greeter", "xdg-utils")


def digest(path: Path) -> str:
    return hashlib.sha256(path.read_bytes()).hexdigest()


def validate_fixture(root: Path, catalog: JsonObject, manifest: JsonObject) -> None:
    require_keys(catalog, {"schema", "ports_commit", "pkgbase_commit", "ports_archive_sha256", "pkgbase_archive_sha256", "roots", "packages"}, "catalog")
    if catalog["schema"] != "aeacus-freebsd-source-catalog-v1" or catalog["ports_commit"] != PORTS_COMMIT or catalog["pkgbase_commit"] != PKGBASE_COMMIT: raise ContractError(detail="source catalog identity mismatch")
    if digest(root / "sources/ports.tar") != require_hash(catalog["ports_archive_sha256"], "catalog.ports_archive_sha256"): raise ContractError(detail="ports archive mismatch")
    if digest(root / "sources/pkgbase.tar") != require_hash(catalog["pkgbase_archive_sha256"], "catalog.pkgbase_archive_sha256"): raise ContractError(detail="pkgbase archive mismatch")
    roots = [require_object(value, "catalog.roots[]") for value in require_array(catalog["roots"], "catalog.roots")]
    keys = [(str(value["release"]), str(value["arch"]), str(value["name"]), str(value["source"])) for value in roots]
    expected = sorted([(release, arch, name, "ports") for release in ("14.4", "15.1") for arch in ("arm64", "x86-64") for name in ROOT_NAMES] + [("15.1", arch, "FreeBSD-runtime", "pkgbase") for arch in ("arm64", "x86-64")])
    if keys != expected or len(set(keys)) != len(keys): raise ContractError(detail="root inventory mismatch")
    packages = [require_object(value, "catalog.packages[]") for value in require_array(catalog["packages"], "catalog.packages")]
    names = {(str(value["release"]), str(value["arch"]), str(value["name"])) for value in packages}
    for value in packages:
        require_keys(value, {"release", "arch", "name", "version", "origin", "source", "source_archive_sha256", "package_sha256", "compact_manifest_sha256", "dependencies", "path", "compact_manifest_path", "bytes"}, "catalog.packages[]")
        package_path = root / str(value["path"])
        compact_path = root / str(value["compact_manifest_path"])
        if not package_path.is_file() or package_path.stat().st_size != require_int(value["bytes"], "catalog.packages[].bytes") or digest(package_path) != require_hash(value["package_sha256"], "catalog.packages[].package_sha256"): raise ContractError(detail="package bytes mismatch")
        if not compact_path.is_file() or digest(compact_path) != require_hash(value["compact_manifest_sha256"], "catalog.packages[].compact_manifest_sha256"): raise ContractError(detail="compact manifest mismatch")
        package = parse_compact_manifest(compact_path.read_bytes(), package_path)
        expected_identity = str(value["name"]), str(value["version"]), str(value["origin"])
        if (package.name, package.version, package.origin) != expected_identity: raise ContractError(detail="compact manifest identity mismatch")
        expected_dependencies = tuple((str(row["name"]), str(row["origin"]), str(row["version"])) for row in [require_object(item, "dependency") for item in require_array(value["dependencies"], "dependencies")])
        if package.dependencies != expected_dependencies: raise ContractError(detail="compact manifest dependencies mismatch")
        for dep in require_array(value["dependencies"], "catalog.packages[].dependencies"):
            dependency = require_object(dep, "dependency")
            require_keys(dependency, {"name", "origin", "version"}, "dependency")
            if (str(value["release"]), str(value["arch"]), str(dependency["name"])) not in names: raise ContractError(detail="unreachable dependency")
    reachable: set[tuple[str, str, str]] = set()
    pending = [(release, arch, name) for release, arch, name, _ in keys]
    by_name = {(str(value["release"]), str(value["arch"]), str(value["name"])): value for value in packages}
    while pending:
        key = pending.pop()
        if key in reachable: continue
        package = by_name.get(key)
        if package is None: raise ContractError(detail=f"missing package closure row: {key}")
        reachable.add(key)
        for dependency in require_array(package["dependencies"], "dependencies"):
            row = require_object(dependency, "dependency")
            pending.append((key[0], key[1], str(row["name"])))
    if reachable != set(by_name): raise ContractError(detail="extra package outside root closure")
    require_keys(manifest, {"schema", "head_sha", "source_date_epoch", "builder_images", "source_catalog_sha256", "assets"}, "manifest")
    if manifest["schema"] != "aeacus-freebsd-fixture-build-v1": raise ContractError(detail="fixture manifest schema mismatch")
    require_hash(manifest["head_sha"], "manifest.head_sha", 40)
    require_int(manifest["source_date_epoch"], "manifest.source_date_epoch")
    if digest(root / "source-catalog.json") != require_hash(manifest["source_catalog_sha256"], "manifest.source_catalog_sha256"): raise ContractError(detail="catalog hash mismatch")
    assets = [require_object(value, "manifest.assets[]") for value in require_array(manifest["assets"], "manifest.assets")]
    if len(assets) != len(packages): raise ContractError(detail="asset/package cardinality mismatch")

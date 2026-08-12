#!/usr/bin/env python3
# /// script
# requires-python = ">=3.12"
# dependencies = []
# ///
# ─── How to run ───
# uv run misc/tests/freebsd/package_select.py SOURCE RELEASE ARCH SOURCE_SHA ROOTS RAW OUTPUT

from __future__ import annotations

import hashlib
import json
import shutil
import subprocess
import sys
from dataclasses import dataclass
from pathlib import Path

from misc.tests.freebsd.json_contract import ContractError, JsonObject, JsonValue, canonical_bytes, unique_object


@dataclass(frozen=True, slots=True)
class Package:
    archive: Path
    manifest: bytes
    name: str
    version: str
    origin: str
    dependencies: tuple[tuple[str, str, str], ...]


def required_text(value: JsonValue, field: str) -> str:
    if not isinstance(value, str) or not value:
        raise ContractError(detail=f"invalid package field: {field}")
    return value


def parse_dependencies(value: JsonValue) -> tuple[tuple[str, str, str], ...]:
    if value is None:
        return ()
    if not isinstance(value, dict):
        raise ContractError(detail="invalid package dependencies")
    rows: list[tuple[str, str, str]] = []
    for name, raw in value.items():
        if not isinstance(raw, dict) or set(raw) != {"origin", "version"}:
            raise ContractError(detail=f"invalid dependency: {name}")
        rows.append((name, required_text(raw["origin"], "origin"), required_text(raw["version"], "version")))
    return tuple(sorted(rows))


def read_package(path: Path) -> Package:
    result = subprocess.run(["tar", "--zstd", "-xOf", path, "+COMPACT_MANIFEST"], check=False, capture_output=True)
    if result.returncode != 0:
        result = subprocess.run(["tar", "-xJOf", path, "+COMPACT_MANIFEST"], check=True, capture_output=True)
    return parse_compact_manifest(result.stdout, path)


def parse_compact_manifest(manifest: bytes, path: Path) -> Package:
    try:
        value = json.loads(manifest, object_pairs_hook=unique_object)
    except (json.JSONDecodeError, UnicodeDecodeError) as error:
        raise ContractError(detail=f"malformed compact manifest: {path}") from error
    if not isinstance(value, dict):
        raise ContractError(detail=f"invalid compact manifest: {path}")
    return Package(path, manifest, required_text(value.get("name"), "name"), required_text(value.get("version"), "version"), required_text(value.get("origin"), "origin"), parse_dependencies(value.get("deps")))


def sha256(data: bytes) -> str:
    return hashlib.sha256(data).hexdigest()


def select(packages: tuple[Package, ...], roots: tuple[str, ...]) -> tuple[Package, ...]:
    by_name: dict[str, Package] = {}
    for package in packages:
        if package.name in by_name:
            raise ContractError(detail=f"duplicate package name: {package.name}")
        by_name[package.name] = package
    pending = list(roots)
    selected: dict[str, Package] = {}
    while pending:
        name = pending.pop()
        if name in selected:
            continue
        package = by_name.get(name)
        if package is None:
            raise ContractError(detail=f"missing root/dependency package: {name}")
        selected[name] = package
        for dep_name, dep_origin, dep_version in package.dependencies:
            dependency = by_name.get(dep_name)
            if dependency is None or (dependency.origin, dependency.version) != (dep_origin, dep_version):
                raise ContractError(detail=f"dependency mismatch: {package.name} -> {dep_name}")
            pending.append(dep_name)
    return tuple(sorted(selected.values(), key=lambda package: (package.origin, package.name, package.version)))


def main(arguments: list[str]) -> int:
    if len(arguments) != 7:
        raise ContractError(detail="usage: package_select.py SOURCE RELEASE ARCH SOURCE_SHA ROOTS RAW OUTPUT")
    source, release, arch, source_hash, roots_text, raw_text, output_text = arguments
    roots = tuple(roots_text.split(","))
    raw = Path(raw_text)
    output = Path(output_text)
    packages = tuple(read_package(path) for path in sorted(raw.glob("*.pkg")))
    selected = select(packages, roots)
    package_dir = output / "packages"
    manifest_dir = output / "compact-manifests"
    package_dir.mkdir(parents=True)
    manifest_dir.mkdir(parents=True)
    rows: list[JsonValue] = []
    for package in selected:
        destination = package_dir / package.archive.name
        shutil.copyfile(package.archive, destination)
        compact_path = manifest_dir / f"{package.archive.name}.compact-manifest"
        compact_path.write_bytes(package.manifest)
        rows.append({
            "release": release, "arch": arch, "name": package.name,
            "version": package.version, "origin": package.origin, "source": source,
            "source_archive_sha256": source_hash,
            "package_sha256": sha256(destination.read_bytes()),
            "compact_manifest_sha256": sha256(package.manifest),
            "dependencies": [{"name": name, "origin": origin, "version": version} for name, origin, version in package.dependencies],
            "filename": package.archive.name, "bytes": destination.stat().st_size,
        })
    fragment: JsonObject = {"release": release, "arch": arch, "source": source, "roots": list(roots), "packages": rows}
    (output / "fragment.json").write_bytes(canonical_bytes(fragment))
    return 0


if __name__ == "__main__":
    raise SystemExit(main(sys.argv[1:]))

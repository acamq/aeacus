#!/usr/bin/env python3
# /// script
# requires-python = ">=3.12"
# dependencies = []
# ///
# ─── How to run ───
# uv run misc/tests/freebsd/validate.py abi INPUT.json

from __future__ import annotations

import sys
from pathlib import Path
from typing import Final

from misc.tests.freebsd.json_contract import ContractError, JsonObject, canonical_bytes, load_document
from misc.tests.freebsd.validate_abi import validate_abi_document
from misc.tests.freebsd.validate_fixture import validate_fixture
from misc.tests.freebsd.validate_metadata import validate_metadata_document

USAGE: Final = "usage: validate.py abi INPUT.json"
IMAGE_ROWS: Final = (
    ("14.4", "arm64", "freebsd-14.4-arm64.qcow2", "https://github.com/cross-platform-actions/freebsd-builder/releases/download/v0.15.0/freebsd-14.4-arm64.qcow2", "345243648", "78fa20cab5254755357f2ee71196138db23ce1a95674c9f34beed44dee1391a6"),
    ("14.4", "x86-64", "freebsd-14.4-x86-64.qcow2", "https://github.com/cross-platform-actions/freebsd-builder/releases/download/v0.15.0/freebsd-14.4-x86-64.qcow2", "377154048", "a124dfec63f9a00b6f90f13d96a40f124b33bd8a92f38391efd33270bd7000c3"),
    ("15.1", "arm64", "freebsd-15.1-arm64.qcow2", "https://github.com/cross-platform-actions/freebsd-builder/releases/download/v0.15.0/freebsd-15.1-arm64.qcow2", "358350848", "5dee8bedce8b1b3a53776f8f60d875b0dd0d696e1258ffcf696885b0dec39118"),
    ("15.1", "x86-64", "freebsd-15.1-x86-64.qcow2", "https://github.com/cross-platform-actions/freebsd-builder/releases/download/v0.15.0/freebsd-15.1-x86-64.qcow2", "390723584", "d7045aa82b38f43af0ab778c6a89725703f5223d04a1d7c3b52bf66b88f164fb"),
)


def validate_image_lock(path: Path) -> None:
    raw = path.read_bytes()
    if raw.startswith(b"\xef\xbb\xbf") or not raw.endswith(b"\n") or b"\r" in raw:
        raise ContractError(detail="non-canonical image lock framing")
    lines = raw.decode("utf-8").splitlines()
    if lines[:2] != ["# aeacus-freebsd-images-v1", "release\tarch\tfilename\turl\tbytes\tsha256"]:
        raise ContractError(detail="wrong image lock header")
    rows = tuple(tuple(line.split("\t")) for line in lines[2:])
    if rows != IMAGE_ROWS:
        raise ContractError(detail="wrong image lock rows/order")


def validate_document(kind: str, document: JsonObject) -> None:
    match kind:
        case "abi":
            validate_abi_document(document)
        case "metadata":
            validate_metadata_document(document)
        case _:
            raise ContractError(detail=f"unknown validation kind: {kind}")


def main(arguments: list[str]) -> int:
    if len(arguments) != 2:
        print(USAGE, file=sys.stderr)
        return 2
    try:
        path = Path(arguments[1])
        if arguments[0] == "fixture":
            catalog_path = path / "source-catalog.json"
            manifest_path = path / "fixture-build.json"
            catalog = load_document(catalog_path)
            manifest = load_document(manifest_path)
            validate_fixture(path, catalog, manifest)
            for source_path, document in ((catalog_path, catalog), (manifest_path, manifest)):
                if source_path.read_bytes() != canonical_bytes(document): raise ContractError(detail=f"non-canonical JSON: {source_path}")
            return 0
        document = load_document(path)
        validate_document(arguments[0], document)
        if path.read_bytes() != canonical_bytes(document):
            raise ContractError(detail=f"non-canonical JSON: {path}")
    except (ContractError, OSError) as error:
        print(error, file=sys.stderr)
        return 1
    return 0


if __name__ == "__main__":
    raise SystemExit(main(sys.argv[1:]))

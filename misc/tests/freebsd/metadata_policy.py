#!/usr/bin/env python3
# /// script
# requires-python = ">=3.12"
# dependencies = []
# ///
# ─── How to run ───
# python3 misc/tests/freebsd/metadata_policy.py PATH [POLICY]

from __future__ import annotations

import sys
from dataclasses import dataclass
from enum import StrEnum
from typing import Final


class ContentHashPolicy(StrEnum):
    SHA256 = "sha256"
    STAT_ONLY = "stat-only"


STAT_ONLY_PATHS: Final = frozenset({"/etc/master.passwd"})


@dataclass(frozen=True, slots=True)
class PolicyError(Exception):
    detail: str

    def __str__(self) -> str:
        return self.detail


def policy_for(path: str) -> ContentHashPolicy:
    return ContentHashPolicy.STAT_ONLY if path in STAT_ONLY_PATHS else ContentHashPolicy.SHA256


def parse_policy(path: str, value: str) -> ContentHashPolicy:
    try:
        policy = ContentHashPolicy(value)
    except ValueError as error:
        raise PolicyError(detail=f"unknown content hash policy: {value}") from error
    if policy is ContentHashPolicy.STAT_ONLY and path not in STAT_ONLY_PATHS:
        raise PolicyError(detail=f"stat-only path is not explicitly classified: {path}")
    if policy is ContentHashPolicy.SHA256 and path in STAT_ONLY_PATHS:
        raise PolicyError(detail=f"sensitive path requires stat-only policy: {path}")
    return policy


def main(arguments: list[str]) -> int:
    if len(arguments) not in {1, 2}:
        print("usage: metadata_policy.py PATH [POLICY]", file=sys.stderr)
        return 2
    try:
        policy = policy_for(arguments[0]) if len(arguments) == 1 else parse_policy(arguments[0], arguments[1])
    except PolicyError as error:
        print(error, file=sys.stderr)
        return 1
    print(policy.value)
    return 0


if __name__ == "__main__":
    raise SystemExit(main(sys.argv[1:]))

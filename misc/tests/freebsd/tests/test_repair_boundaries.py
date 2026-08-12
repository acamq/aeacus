#!/usr/bin/env python3
# /// script
# requires-python = ">=3.12"
# dependencies = []
# ///
# ─── How to run ───
# uv run misc/tests/freebsd/tests/test_repair_boundaries.py

from __future__ import annotations

import os
import re
import shutil
import sys
import subprocess
import tempfile
from pathlib import Path
from typing import Final

ROOT: Final = Path(__file__).resolve().parents[4]
FREEBSD: Final = ROOT / "misc/tests/freebsd"
sys.path.insert(0, str(ROOT))
DISCOVER: Final = FREEBSD / "ci/discover.sh"
FIXTURES: Final = FREEBSD / "ci/build-fixtures.sh"


def test_static_boundaries() -> None:
    discovery = DISCOVER.read_text(encoding="utf-8")
    fixtures = FIXTURES.read_text(encoding="utf-8")
    assert discovery.count("/etc/master.passwd") == 1
    assert "python" not in discovery and "uv " not in discovery
    assert 'case "$path" in' in discovery
    assert '/etc/master.passwd) policy=stat-only ;;' in discovery
    assert 'if [ "$policy" = stat-only ]' in discovery
    assert "/usr/ports" not in fixtures
    assert "PACKAGES=" in fixtures and "PORTSDIR=" in fixtures and "DISTDIR=" in fixtures


def test_discovery_never_reads_sensitive_content() -> None:
    with tempfile.TemporaryDirectory() as directory:
        root = Path(directory)
        fake_bin = root / "bin"
        fake_bin.mkdir()
        log = root / "sha.log"
        fake_sha = fake_bin / "sha256"
        fake_sha.write_text("#!/bin/sh\nprintf '%s\\n' \"$*\" >> \"$SHA_LOG\"\nprintf '%064d\\n' 0\n", encoding="ascii")
        fake_sha.chmod(0o755)
        env = os.environ | {"PATH": f"{fake_bin}:{os.environ['PATH']}", "SHA_LOG": str(log)}
        discovery = DISCOVER.read_text(encoding="utf-8")
        assert not re.search(r"sha256[^\n]*master\.passwd", discovery)
        assert not log.exists(), "sensitive metadata path reached sha256"


def test_metadata_schema_policy() -> None:
    from misc.tests.freebsd.validate_metadata import validate_path
    from misc.tests.freebsd.json_contract import JsonObject
    sensitive: JsonObject = {"path": "/etc/master.passwd", "present": True, "type": "Regular File", "uid": 0, "gid": 0, "mode": "0600", "nlink": 1, "bytes": 1, "content_hash_policy": "stat-only"}
    assert validate_path(sensitive) == "/etc/master.passwd"
    for damaged in (
        sensitive | {"content_hash_policy": "sha256"},
        {key: value for key, value in sensitive.items() if key != "content_hash_policy"},
        sensitive | {"sha256": "0" * 64},
    ):
        try:
            validate_path(damaged)
        except Exception:
            continue
        raise AssertionError("invalid sensitive metadata policy accepted")
    ordinary = sensitive | {"path": "/etc/passwd"}
    try:
        validate_path(ordinary)
    except Exception:
        pass
    else:
        raise AssertionError("ordinary regular file omitted SHA-256")


def test_discovery_driver_sensitive_and_unreadable_boundaries() -> None:
    with tempfile.TemporaryDirectory() as directory:
        root = Path(directory)
        workspace = root / "workspace"
        shutil.copytree(FREEBSD, workspace / "misc/tests/freebsd")
        paths = workspace / "misc/tests/freebsd/metadata-paths.txt"
        paths.write_text("/etc/master.passwd\n/etc/passwd\n", encoding="utf-8")
        fake_bin = root / "bin"
        fake_bin.mkdir()
        log = root / "sha.log"
        commands = {
            "cc": "#!/bin/sh\nout=\nwhile [ $# -gt 0 ]; do [ \"$1\" = -o ] && { shift; out=$1; }; shift; done\ncp \"$PROBE_FIXTURE\" \"$out\"\nchmod +x \"$out\"\n",
            "freebsd-version": "#!/bin/sh\nprintf '15.1-RELEASE\\n'\n",
            "stat": "#!/bin/sh\nfor last do :; done\nprintf '%s\\tRegular File\\t0\\t0\\t0600\\t1\\t1\\n' \"$last\"\n",
            "sha256": "#!/bin/sh\ncase \"$*\" in *master.passwd*) exit 97;; *'/etc/passwd'*) printf '%s\\n' ordinary >> \"$SHA_LOG\"; exit 98;; *) printf '%064d\\n' 0;; esac\n",
        }
        for name, body in commands.items():
            path = fake_bin / name
            path.write_text(body, encoding="ascii")
            path.chmod(0o755)
        probe = root / "probe"
        probe.write_text("#!/bin/sh\nprintf 'struct_size\\t1088\\np_traced\\t2048\\nki_structsize\\t0\\t4\\t0\\nki_pid\\t72\\t4\\t1\\nki_flag\\t360\\t8\\t0\\nki_tracer\\t1120\\t4\\t1\\nki_start\\t312\\t16\\t0\\n'\n", encoding="ascii")
        probe.chmod(0o755)
        env = os.environ | {"PATH": f"{fake_bin}:{os.environ['PATH']}", "SHA_LOG": str(log), "PROBE_FIXTURE": str(probe), "RELEASE": "15.1", "ARCH": "x86-64", "GITHUB_SHA": "a" * 40, "GITHUB_RUN_ID": "1", "GITHUB_RUN_ATTEMPT": "1"}
        result = subprocess.run(["sh", "misc/tests/freebsd/ci/discover.sh"], cwd=workspace, env=env, check=False, capture_output=True, text=True)
        assert result.returncode == 98, (result.returncode, result.stdout, result.stderr)
        assert log.read_text(encoding="ascii") == "ordinary\n"
        assert "master.passwd" not in result.stdout and "master.passwd" not in result.stderr


def test_fixture_driver_uses_only_writable_outputs() -> None:
    fixtures = FIXTURES.read_text(encoding="utf-8")
    forbidden = ("/usr/ports", "/usr/obj", "sudo ", "doas ", "PACKAGES=/", "DISTDIR=/")
    for value in forbidden:
        assert value not in fixtures
    expected = ('packages="$work/packages"', 'distfiles="$work/distfiles"', 'PORTSDIR="$work/ports"', 'PACKAGES="$packages"', 'DISTDIR="$distfiles"')
    for value in expected:
        assert value in fixtures
    mutation_commands = ("mkdir", "cp", "tar", "make", "rm")
    for line in fixtures.splitlines():
        stripped = line.strip()
        if stripped.startswith(mutation_commands):
            assert any(value in stripped for value in ('"$work', '"$out', '"$packages', '"$distfiles', '"$objdir')) or stripped.startswith("tar -xf sources/")


def test_archive_members_are_safe() -> None:
    def safe(name: str) -> bool:
        path = Path(name)
        return not path.is_absolute() and ".." not in path.parts and "" not in path.parts
    assert safe("usr/ports/Mk/bsd.port.mk")
    for name in ("/etc/passwd", "../escape", "root/../../escape"):
        assert not safe(name)


def test_archive_filter_executes() -> None:
    with tempfile.TemporaryDirectory() as directory:
        root = Path(directory)
        source = root / "source"
        source.mkdir()
        (source / "safe").write_text("fixture", encoding="ascii")
        archive = root / "safe.tar"
        subprocess.run(["tar", "-cf", archive, "safe"], cwd=source, check=True)
        result = subprocess.run(["sh", str(FREEBSD / "ci/extract-source.sh"), str(archive), str(root / "output")], check=False, capture_output=True, text=True)
        assert result.returncode == 0, result.stderr
        assert (root / "output/safe").read_text(encoding="ascii") == "fixture"


def test_guest_shell_uses_base_tools() -> None:
    forbidden = re.compile(r"\b(?:python3?|uv|gawk|bash|sha256sum)\b")
    for script in sorted((FREEBSD / "ci").glob("*.sh")):
        if script.name in {"stage-image.sh", "stop-image.sh"}:
            continue
        text = script.read_text(encoding="utf-8")
        executable_text = "\n".join(line for line in text.splitlines() if "command -v python" not in line)
        assert forbidden.search(executable_text) is None, f"non-base guest command in {script.name}"


def test_awk_variables_avoid_builtin_names() -> None:
    builtins = ("index", "length", "split", "substr", "match", "sub", "gsub", "sprintf")
    for script in sorted((FREEBSD / "ci").glob("*.sh")):
        text = script.read_text(encoding="utf-8")
        for name in builtins:
            assert re.search(rf"\b{name}\s*=", text) is None, f"awk built-in assigned in {script.name}: {name}"


def main() -> int:
    test_static_boundaries()
    test_discovery_never_reads_sensitive_content()
    test_metadata_schema_policy()
    test_discovery_driver_sensitive_and_unreadable_boundaries()
    test_fixture_driver_uses_only_writable_outputs()
    test_archive_members_are_safe()
    test_archive_filter_executes()
    test_guest_shell_uses_base_tools()
    test_awk_variables_avoid_builtin_names()
    print("Task 10 repair boundaries: PASS")
    return 0


if __name__ == "__main__":
    raise SystemExit(main())

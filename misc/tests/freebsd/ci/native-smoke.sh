#!/bin/sh
set -eu

if [ "$#" -ne 1 ]; then
    exit 2
fi

for command_name in sh awk tar stat sha256 find make cc freebsd-version; do
    command -v "$command_name" >/dev/null
done

case "$1" in
    discovery)
        ! command -v python3 >/dev/null 2>&1
        ! command -v python >/dev/null 2>&1
        sensitive_path=/etc/master.passwd
        policy=sha256
        case "$sensitive_path" in
            /etc/master.passwd) policy=stat-only ;;
        esac
        [ "$policy" = stat-only ]
        stat -f '%N\t%HT\t%u\t%g\t%Mp%Lp\t%l\t%z' /etc/master.passwd >/dev/null
        ;;
    fixtures)
        workspace="/tmp/aeacus-native-smoke-$$"
        trap 'rm -rf "$workspace"' EXIT INT TERM
        mkdir "$workspace" "$workspace/source"
        printf fixture > "$workspace/source/safe"
        tar -cf "$workspace/safe.tar" -C "$workspace/source" safe
        misc/tests/freebsd/ci/extract-source.sh "$workspace/safe.tar" "$workspace/output"
        [ "$(cat "$workspace/output/safe")" = fixture ]
        [ -w "$workspace" ]
        ;;
    *) exit 2 ;;
esac

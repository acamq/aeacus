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
        printf 'safe/path\n' | awk '
        {
            count = split($0, parts, "/")
            for (part_number = 1; part_number <= count; part_number++) {
                if (parts[part_number] == ".." || parts[part_number] == "") exit 1
            }
        }'
        workspace="/tmp/aeacus-native-smoke-$$"
        trap 'rm -rf "$workspace"' EXIT INT TERM
        mkdir "$workspace"
        [ -w "$workspace" ]
        ;;
    *) exit 2 ;;
esac

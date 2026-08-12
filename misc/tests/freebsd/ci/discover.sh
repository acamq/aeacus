#!/bin/sh
set -eu

: "${RELEASE:?}" "${ARCH:?}"
: "${GITHUB_SHA:?}" "${GITHUB_RUN_ID:?}" "${GITHUB_RUN_ATTEMPT:?}"

out="out/abi/$RELEASE/$ARCH"
mkdir -p "$out"

cc -std=c11 -Wall -Wextra -Werror -o "$out/kinfo-probe" misc/tests/freebsd/kinfo_probe.c
"$out/kinfo-probe" > "$out/probe.tsv"
freebsd-version -ku > "$out/freebsd-version.txt"
sha256 -q /usr/include/sys/user.h > "$out/user-h.sha256"
sha256 -q /usr/include/sys/proc.h > "$out/proc-h.sha256"
sha256 -q misc/tests/freebsd/kinfo_probe.c > "$out/probe-source.sha256"

while IFS= read -r path; do
    [ -n "$path" ] || continue
    if [ -e "$path" ] || [ -L "$path" ]; then
        stat -f '%N\t%HT\t%u\t%g\t%Mp%Lp\t%l\t%z' "$path"
        if [ -f "$path" ]; then sha256 -q "$path"; else printf '%s\n' -; fi
    else
        printf '%s\tabsent\n' "$path"
    fi
done < misc/tests/freebsd/metadata-paths.txt > "$out/metadata.tsv"

rm -f "$out/kinfo-probe"

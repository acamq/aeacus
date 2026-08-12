#!/bin/sh
set -eu

if [ "$#" -ne 2 ]; then
    echo "usage: extract-source.sh ARCHIVE DESTINATION" >&2
    exit 2
fi

archive=$1
destination=$2
listing="${TMPDIR:-/tmp}/aeacus-archive-members-$$"
trap 'rm -f "$listing"' EXIT INT TERM
tar -tf "$archive" > "$listing"
while IFS= read -r member; do
    case "$member" in
        ''|/*|..|../*|*/..|*/../*|*//* ) exit 1 ;;
    esac
done < "$listing"
mkdir "$destination"
tar -xf "$archive" -C "$destination"

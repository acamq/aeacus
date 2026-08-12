#!/bin/sh
set -eu

if [ "$#" -ne 2 ]; then
    echo "usage: extract-source.sh ARCHIVE DESTINATION" >&2
    exit 2
fi

archive=$1
destination=$2
tar -tf "$archive" | awk '
BEGIN { failed = 0 }
/^\// { failed = 1 }
{
    count = split($0, parts, "/")
    for (index = 1; index <= count; index++) {
        if (parts[index] == ".." || parts[index] == "") failed = 1
    }
}
END { exit failed }
'
mkdir "$destination"
tar -xf "$archive" -C "$destination"

#!/bin/sh
set -eu

: "${RELEASE:?}" "${ARCH:?}"
: "${SOURCE_DATE_EPOCH:?}" "${GITHUB_SHA:?}"

out="out/fixtures/$RELEASE/$ARCH"
work="/tmp/aeacus-fixtures-$GITHUB_RUN_ID-$GITHUB_RUN_ATTEMPT-$RELEASE-$ARCH"
trap 'rm -rf "$work"' EXIT INT TERM
mkdir -p "$out/sources" "$work"

case "$SOURCE_DATE_EPOCH" in *[!0-9]*|'') exit 1 ;; esac
export SOURCE_DATE_EPOCH BATCH=yes DISABLE_VULNERABILITIES=yes PACKAGE_BUILDING=yes

cp sources/ports.tar "$out/sources/ports.tar"
mkdir "$work/ports"
tar -xf sources/ports.tar -C "$work/ports"

rm -rf /usr/ports/packages
mkdir -p /usr/ports/packages
for origin in x11-wm/xfce4 x11/lightdm x11/lightdm-gtk-greeter devel/xdg-utils; do
make -C "$work/ports/$origin" package-recursive
done
mkdir -p "$work/ports-packages"
find "$work/ports/packages" /usr/ports/packages -type f -name '*.pkg' -exec cp '{}' "$work/ports-packages/" ';' 2>/dev/null || true
[ "$(find "$work/ports-packages" -type f -name '*.pkg' | wc -l)" -gt 0 ]
mkdir -p "$out/ports-raw"
cp "$work/ports-packages"/*.pkg "$out/ports-raw/"

if [ "$RELEASE" = 15.1 ]; then
    cp sources/pkgbase.tar "$out/sources/pkgbase.tar"
    mkdir "$work/src"
    tar -xf sources/pkgbase.tar -C "$work/src"
    make -C "$work/src" -j2 buildworld
    make -C "$work/src" packages
    mkdir -p "$work/pkgbase-packages"
    find "$work/src" /usr/obj -type f -name '*.pkg' -exec cp '{}' "$work/pkgbase-packages/" ';' 2>/dev/null || true
    [ "$(find "$work/pkgbase-packages" -type f -name '*.pkg' | wc -l)" -gt 0 ]
    mkdir -p "$out/pkgbase-raw"
    cp "$work/pkgbase-packages"/*.pkg "$out/pkgbase-raw/"
fi

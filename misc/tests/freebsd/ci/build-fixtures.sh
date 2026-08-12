#!/bin/sh
set -eu

: "${RELEASE:?}" "${ARCH:?}"
: "${SOURCE_DATE_EPOCH:?}" "${GITHUB_SHA:?}"

artifact_arch=$ARCH
unset ARCH
out="out/fixtures/$RELEASE/$artifact_arch"
work="/tmp/aeacus-fixtures-$GITHUB_RUN_ID-$GITHUB_RUN_ATTEMPT-$RELEASE-$artifact_arch"
trap 'rm -rf "$work"' EXIT INT TERM
mkdir -p "$out/sources" "$work"

case "$SOURCE_DATE_EPOCH" in *[!0-9]*|'') exit 1 ;; esac
export SOURCE_DATE_EPOCH BATCH=yes DISABLE_VULNERABILITIES=yes PACKAGE_BUILDING=yes

cp sources/ports.tar "$out/sources/ports.tar"
misc/tests/freebsd/ci/extract-source.sh sources/ports.tar "$work/ports"

packages="$work/packages"
distfiles="$work/distfiles"
sysroot="$work/sysroot"
localbase="$sysroot/usr/local"
mkdir -p "$packages" "$distfiles" "$localbase"
roots="x11-wm/xfce4 x11/lightdm x11/lightdm-gtk-greeter devel/xdg-utils"
: > "$work/staged-packages"
export PATH="$localbase/bin:$localbase/sbin:$PATH"
export PKG_CONFIG_SYSROOT_DIR="$sysroot"
export PKG_CONFIG_LIBDIR="$localbase/libdata/pkgconfig:$localbase/lib/pkgconfig:$localbase/share/pkgconfig"
stage_dependency() (
    dependency=$1
    [ -n "$dependency" ] || return
    case "$(cat "$work/staged-packages")" in *"|$dependency|"*) return ;; esac
    printf '|%s|\n' "$dependency" >> "$work/staged-packages"
    for child in $(make -C "$dependency" build-depends-list run-depends-list PORTSDIR="$work/ports" LOCALBASE="$localbase"); do
        stage_dependency "$child"
    done
    make -C "$dependency" package-noinstall PORTSDIR="$work/ports" PACKAGES="$packages" DISTDIR="$distfiles" WRKDIRPREFIX="$work/wrk" LOCALBASE="$localbase" PREFIX=/usr/local INSTALL_AS_USER=yes NO_DEPENDS=yes CC="cc -I$localbase/include -L$localbase/lib" CXX="c++ -I$localbase/include -L$localbase/lib"
    package_file=$(make -C "$dependency" -V PKGFILE PORTSDIR="$work/ports" PACKAGES="$packages" DISTDIR="$distfiles" WRKDIRPREFIX="$work/wrk" LOCALBASE="$localbase" PREFIX=/usr/local INSTALL_AS_USER=yes)
    tar -xf "$package_file" -C "$sysroot" --exclude +COMPACT_MANIFEST --exclude +MANIFEST
)
for origin in $roots; do
    for dependency in $(make -C "$work/ports/$origin" build-depends-list run-depends-list PORTSDIR="$work/ports" LOCALBASE="$localbase"); do
        stage_dependency "$dependency"
    done
done
for origin in $roots; do
    make -C "$work/ports/$origin" package-noinstall PORTSDIR="$work/ports" PACKAGES="$packages" DISTDIR="$distfiles" WRKDIRPREFIX="$work/wrk" LOCALBASE="$localbase" PREFIX=/usr/local INSTALL_AS_USER=yes NO_DEPENDS=yes CC="cc -I$localbase/include -L$localbase/lib" CXX="c++ -I$localbase/include -L$localbase/lib"
done
mkdir -p "$work/ports-packages"
find "$packages" -type f -name '*.pkg' -exec cp '{}' "$work/ports-packages/" ';'
[ "$(find "$work/ports-packages" -type f -name '*.pkg' | wc -l)" -gt 0 ]
mkdir -p "$out/ports-raw"
cp "$work/ports-packages"/*.pkg "$out/ports-raw/"

if [ "$RELEASE" = 15.1 ]; then
    cp sources/pkgbase.tar "$out/sources/pkgbase.tar"
    misc/tests/freebsd/ci/extract-source.sh sources/pkgbase.tar "$work/src"
    objdir="$work/obj"
    mkdir "$objdir"
    make -C "$work/src" -j2 buildworld MAKEOBJDIRPREFIX="$objdir"
    make -C "$work/src" packages MAKEOBJDIRPREFIX="$objdir"
    mkdir -p "$work/pkgbase-packages"
    find "$work/src" "$objdir" -type f -name '*.pkg' -exec cp '{}' "$work/pkgbase-packages/" ';'
    [ "$(find "$work/pkgbase-packages" -type f -name '*.pkg' | wc -l)" -gt 0 ]
    mkdir -p "$out/pkgbase-raw"
    cp "$work/pkgbase-packages"/*.pkg "$out/pkgbase-raw/"
fi

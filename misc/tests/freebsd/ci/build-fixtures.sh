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
for origin in $roots; do
    make -C "$work/ports/$origin" all-depends-list PORTSDIR="$work/ports" PACKAGES="$packages" DISTDIR="$distfiles" WRKDIRPREFIX="$work/wrk" INSTALL_AS_USER=yes
done | tail -r > "$work/dependencies"
: > "$work/staged-packages"
while IFS= read -r dependency; do
    [ -n "$dependency" ] || continue
    make -C "$dependency" package-noinstall PORTSDIR="$work/ports" PACKAGES="$packages" DISTDIR="$distfiles" WRKDIRPREFIX="$work/wrk" LOCALBASE="$localbase" PREFIX=/usr/local INSTALL_AS_USER=yes NO_DEPENDS=yes CC="cc -I$localbase/include -L$localbase/lib" CXX="c++ -I$localbase/include -L$localbase/lib"
    package_file=$(make -C "$dependency" -V PKGFILE PORTSDIR="$work/ports" PACKAGES="$packages" DISTDIR="$distfiles" WRKDIRPREFIX="$work/wrk" LOCALBASE="$localbase" PREFIX=/usr/local INSTALL_AS_USER=yes)
    case "$(cat "$work/staged-packages")" in
        *"|$package_file|"*) ;;
        *) tar -xf "$package_file" -C "$sysroot" --exclude +COMPACT_MANIFEST --exclude +MANIFEST; printf '|%s|\n' "$package_file" >> "$work/staged-packages" ;;
    esac
done < "$work/dependencies"
export PATH="$localbase/bin:$localbase/sbin:$PATH"
export PKG_CONFIG_SYSROOT_DIR="$sysroot"
export PKG_CONFIG_LIBDIR="$localbase/libdata/pkgconfig:$localbase/lib/pkgconfig:$localbase/share/pkgconfig"
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

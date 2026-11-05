#!/usr/bin/env bash
# Shared Windows x64 cross-compilation helpers; stages pin their own sources.
set -euo pipefail
script_dir=$(CDPATH= cd -- "$(dirname -- "${BASH_SOURCE[0]}")" && pwd)
source "$script_dir/fetch.sh"
stage=$1
export PREFIX=/opt/windows
export TARGET=x86_64-w64-mingw32
export PKG_CONFIG_LIBDIR="$PREFIX/lib/pkgconfig"
export PKG_CONFIG_PATH="$PKG_CONFIG_LIBDIR"
export CFLAGS='-O2'
export CXXFLAGS='-O2'
jobs=${BUILD_JOBS:-16}
if [[ ! "$jobs" =~ ^[1-9][0-9]*$ ]]; then
    echo "BUILD_JOBS must be a positive integer, got: $jobs" >&2
    exit 2
fi
mkdir -p /build "$PREFIX" /output/bin /output/licenses
cd /build

cmake_cross() {
    cmake -G Ninja -DCMAKE_SYSTEM_NAME=Windows -DCMAKE_SYSTEM_PROCESSOR=AMD64 \
        -DCMAKE_C_COMPILER="$TARGET-gcc" -DCMAKE_CXX_COMPILER="$TARGET-g++" \
        -DCMAKE_RC_COMPILER="$TARGET-windres" -DCMAKE_BUILD_TYPE=Release \
        -DCMAKE_INSTALL_PREFIX="$PREFIX" -DCMAKE_INSTALL_LIBDIR=lib \
        -DBUILD_SHARED_LIBS=OFF "$@"
}

case "$stage" in
    aom|svt|x265|fdk|nvcodec|ffmpeg) source "$script_dir/$stage.sh" ;;
    *) echo "Unknown build stage: $stage" >&2; exit 2 ;;
esac

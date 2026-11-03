#!/usr/bin/env bash
# Windows x64 production encoders. Versions match ffmpeg-build-script v9.0.10.
set -euo pipefail
script_dir=$(CDPATH= cd -- "$(dirname -- "${BASH_SOURCE[0]}")" && pwd)
stage=$1
export PREFIX=/opt/windows
export TARGET=x86_64-w64-mingw32
export PKG_CONFIG_LIBDIR="$PREFIX/lib/pkgconfig"
export PKG_CONFIG_PATH="$PKG_CONFIG_LIBDIR"
export CFLAGS='-O2'
export CXXFLAGS='-O2'
jobs=${BUILD_JOBS:-2}
mkdir -p /build "$PREFIX" /output/bin /output/licenses
cd /build

fetch() {
    local name=$1 url=$2 checksum=$3
    curl --fail --location --retry 5 --connect-timeout 30 "$url" -o "$name.tar.gz"
    echo "$checksum  $name.tar.gz" | sha256sum --check
    mkdir -p "$name"
    tar -xf "$name.tar.gz" -C "$name" --strip-components=1
    rm "$name.tar.gz"
}

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

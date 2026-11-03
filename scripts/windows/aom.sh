#!/usr/bin/env bash
# Sourced by build-ffmpeg.sh.
fetch aom https://storage.googleapis.com/aom-releases/libaom-3.14.1.tar.gz 44bf90dbd23e734d50e70a8c41c285193922938bd0d3bc2ee56764d181d55ef5
cmake_cross -S aom -B aom-build -DENABLE_TESTS=OFF -DENABLE_EXAMPLES=OFF -DENABLE_TOOLS=OFF -DENABLE_DOCS=OFF -DAOM_TARGET_CPU=x86_64
cmake --build aom-build -j "$jobs"
cmake --install aom-build
cp aom/LICENSE /output/licenses/aom.txt

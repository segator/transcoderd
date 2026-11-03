#!/usr/bin/env bash
# Sourced by build-ffmpeg.sh.
fetch svt https://gitlab.com/AOMediaCodec/SVT-AV1/-/archive/v4.2.0/SVT-AV1-v4.2.0.tar.gz c7b13c4a84bd3751aa35fcc72be13e6875467e7c2216879251a486e5b1e4e740
cmake_cross -S svt -B svt-build -DENABLE_SHARED=OFF -DBUILD_APPS=OFF -DBUILD_TESTING=OFF
cmake --build svt-build -j "$jobs"
cmake --install svt-build
cp svt/LICENSE* /output/licenses/

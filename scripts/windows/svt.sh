#!/usr/bin/env bash
# Sourced by build-ffmpeg.sh.
fetch svt https://gitlab.com/AOMediaCodec/SVT-AV1/-/archive/v4.3.0/SVT-AV1-v4.3.0.tar.gz 95ce8fc2381ba707e65ac8b9b2e165915bfbe9cd4d2f7b0f58d64ce31bd98f6d
cmake_cross -S svt -B svt-build -DENABLE_SHARED=OFF -DBUILD_APPS=OFF -DBUILD_TESTING=OFF
cmake --build svt-build -j "$jobs"
cmake --install svt-build
cp svt/LICENSE* /output/licenses/

#!/usr/bin/env bash
# Sourced by build-ffmpeg.sh.
fetch x265 https://bitbucket.org/multicoreware/x265_git/get/b81f650e21e8aacbe6a9ad04ce14aefc05b932c0.tar.gz 540de59b5004274f70a4fe229e86c7602d49b5b1a96112fd95a9dcdb5c9f1dc9
# The worker's supported HEVC profile is main10.
cmake_cross -S x265/source -B x265-build -DENABLE_SHARED=OFF -DENABLE_CLI=OFF -DHIGH_BIT_DEPTH=ON -DENABLE_HDR10_PLUS=ON -DCMAKE_CXX_FLAGS='-O2 -include cstdint'
cmake --build x265-build -j "$jobs"
cmake --install x265-build
cp x265/COPYING /output/licenses/x265.txt
sed -i "s/-lgcc_s/-lgcc_eh/g" "$PREFIX/lib/pkgconfig/x265.pc"

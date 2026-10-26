#!/usr/bin/env bash
# Sourced by build-ffmpeg.sh.
fetch nvcodec https://github.com/FFmpeg/nv-codec-headers/releases/download/n13.1.15.0/nv-codec-headers-13.1.15.0.tar.gz 52532ceade3d5c1af62624986f13cf01b63c910576b08c0c278756c5e4b41ad0
make -C nvcodec PREFIX="$PREFIX" install
cp nvcodec/include/ffnvcodec/nvEncodeAPI.h /output/licenses/nvEncodeAPI.h

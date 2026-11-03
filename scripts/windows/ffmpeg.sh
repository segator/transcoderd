#!/usr/bin/env bash
# Sourced by build-ffmpeg.sh.
fetch ffmpeg https://ffmpeg.org/releases/ffmpeg-9.0.2.tar.gz 84960df915059e8754fef2cd7c9afeb614062b1b5458ec471eecee619ee04e98
cd ffmpeg
./configure --prefix=/output --target-os=mingw32 --arch=x86_64 \
    --enable-cross-compile --cross-prefix="$TARGET-" --pkg-config=pkg-config \
    --pkg-config-flags=--static --disable-autodetect --disable-shared --enable-static \
    --disable-debug --disable-doc --disable-ffplay --enable-gpl --enable-nonfree \
    --enable-libaom --enable-libsvtav1 --enable-libx265 --enable-libfdk-aac \
    --enable-ffnvcodec --enable-nvenc --enable-nvdec --enable-cuvid \
    --extra-cflags="-I$PREFIX/include" --extra-ldflags="-L$PREFIX/lib -static" \
    --extra-libs='-lstdc++ -lwinpthread -lws2_32' || { tail -n 100 ffbuild/config.log >&2; exit 1; }
make -j "$jobs"
make install
"$TARGET-strip" /output/bin/ffmpeg.exe /output/bin/ffprobe.exe
cp LICENSE.md /output/licenses/ffmpeg.txt
printf '%s\n' 'FFmpeg 9.0.2; aom 3.14.1; SVT-AV1 4.2.0; x265 b81f650e21e8aacbe6a9ad04ce14aefc05b932c0; FDK AAC 2.0.3; nv-codec-headers 13.1.15.0' > /output/toolchain.txt
# Keep only runtime artifacts; avoid exporting headers and static libraries.
rm -rf /output/include /output/lib /output/share

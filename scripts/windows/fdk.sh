#!/usr/bin/env bash
# Sourced by build-ffmpeg.sh.
fetch fdk 'https://downloads.sourceforge.net/project/opencore-amr/fdk-aac/fdk-aac-2.0.3.tar.gz' 829b6b89eef382409cda6857fd82af84fabb63417b08ede9ea7a553f811cb79e
cd fdk
./configure --host="$TARGET" --prefix="$PREFIX" --disable-shared --enable-static
make -j "$jobs"
make install
cp NOTICE /output/licenses/fdk-aac.txt

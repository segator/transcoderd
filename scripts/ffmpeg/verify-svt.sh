#!/usr/bin/env bash
set -euo pipefail
bin=$1
version=$2
work=$(mktemp -d)
trap 'rm -rf "$work"' EXIT
"$bin/ffmpeg" -version >"$work/version.log"
grep -Fq 'ffmpeg version 9.0.2' "$work/version.log"
"$bin/ffmpeg" -hide_banner -encoders >"$work/encoders.log"
for encoder in libsvtav1 libaom-av1 av1_nvenc libx265 libfdk_aac; do
    grep -Eq " $encoder " "$work/encoders.log"
done
"$bin/ffmpeg" -hide_banner -nostdin -f lavfi -i testsrc2=size=128x128:rate=4:duration=1 \
    -pix_fmt yuv420p10le -c:v libsvtav1 -preset 4 -crf 26 -y "$work/av1.mkv" >"$work/encode.log" 2>&1
cat "$work/encode.log"
grep -Eq "SVT-AV1 Encoder Lib v${version//./\\.}([[:space:]]|$)" "$work/encode.log"
"$bin/ffprobe" -v error -select_streams v:0 -show_entries stream=codec_name,pix_fmt \
    -of default=noprint_wrappers=1 "$work/av1.mkv" >"$work/probe.log"
grep -Fxq 'codec_name=av1' "$work/probe.log"
grep -Fxq 'pix_fmt=yuv420p10le' "$work/probe.log"
"$bin/ffmpeg" -v error -xerror -i "$work/av1.mkv" -f null -

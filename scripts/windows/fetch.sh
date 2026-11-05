#!/usr/bin/env bash
# Sourced by build-ffmpeg.sh; keep downloads verifiable and safe to retry.
fetch() (
    set -euo pipefail
    local name=$1 url=$2 checksum=$3 archive
    archive=$(mktemp "$name.tar.gz.XXXXXX")
    trap 'rm -f -- "$archive"' EXIT
    # --retry alone excludes TLS resets (35) and incomplete transfers (18).
    # Regular --output files are rewound by curl before each retry.
    curl --fail --location --retry 5 --retry-all-errors --retry-max-time 300 \
        --connect-timeout 30 --max-time 120 "$url" --output "$archive"
    printf '%s  %s\n' "$checksum" "$archive" | sha256sum --check
    mkdir -p "$name"
    tar -xf "$archive" -C "$name" --strip-components=1
)

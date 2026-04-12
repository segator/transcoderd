# Pre-built builder images — pulled from the registry instead of rebuilding.
# Override with --build-arg to rebuild from scratch (or use --target builder-ffmpeg).
# To publish new builder images: make publish-builder-ffmpeg / make publish-builder-pgs
ARG FFMPEG_IMAGE=ghcr.io/segator/transcoderd:builder-ffmpeg
ARG PGS_IMAGE=ghcr.io/segator/transcoderd:builder-pgs

# =============================================================================
# Stage: builder-ffmpeg
# Compiles FFmpeg from source (~50min). Published as a standalone image so CI
# never has to rebuild it. To rebuild: make publish-builder-ffmpeg
# =============================================================================
FROM ubuntu:noble-20251013 AS builder-ffmpeg

ENV DEBIAN_FRONTEND=noninteractive

RUN apt-get update \
    && apt-get -y --no-install-recommends install git build-essential curl ca-certificates libva-dev \
        python3 python-is-python3 ninja-build meson \
    && apt-get clean && rm -rf /var/lib/apt/lists/* /tmp/* /var/tmp/* /usr/share/doc/* \
    && update-ca-certificates

WORKDIR /app

ARG FFMPEG_BUILD_SCRIPT_VERSION=v1.59
RUN git clone --depth 1 --branch ${FFMPEG_BUILD_SCRIPT_VERSION} https://github.com/markus-perl/ffmpeg-build-script.git && \
    cd ffmpeg-build-script && \
    SKIPINSTALL=yes ./build-ffmpeg --build --enable-gpl-and-non-free && \
    rm -rf packages

# =============================================================================
# Stage: builder-pgs
# Builds PgsToSrt .NET tool + downloads tessdata (~5min). Also published as a
# standalone image. To rebuild: make publish-builder-pgs
# =============================================================================
FROM mcr.microsoft.com/dotnet/sdk:8.0.419 AS builder-pgs

WORKDIR /src
ARG tessdata_version=ced78752cc61322fb554c280d13360b35b8684e4
ARG pgstosrt_version=ef11919491b5c98f9dcdaf13d721596e60efb7ed

RUN apt-get -y update && \
    apt-get -y upgrade && \
    apt-get -y install \
        automake ca-certificates g++ libtool libtesseract-dev \
        make pkg-config wget unzip libc6-dev && \
    apt-get clean && rm -rf /var/lib/apt/lists/*

RUN wget -O tessdata.zip "https://github.com/tesseract-ocr/tessdata/archive/${tessdata_version}.zip" && \
    unzip tessdata.zip && rm tessdata.zip && \
    mv tessdata-${tessdata_version} tessdata

RUN wget -O pgstosrt.zip "https://github.com/Tentacule/PgsToSrt/archive/${pgstosrt_version}.zip" && \
    unzip pgstosrt.zip && rm pgstosrt.zip && \
    cd PgsToSrt-${pgstosrt_version}/src && \
    dotnet restore && \
    dotnet publish -c Release -f net8.0 -o /src/PgsToSrt/out

# =============================================================================
# Stage: base
# Runtime base with FFmpeg binaries from the pre-built image.
# =============================================================================
FROM ${FFMPEG_IMAGE} AS ffmpeg-bins

FROM debian:trixie-20241223-slim AS base
RUN apt-get update \
    && apt-get install -y ca-certificates mkvtoolnix libva-drm2 wget \
    && rm -rf /var/lib/apt/lists/*

COPY --from=ffmpeg-bins /app/ffmpeg-build-script/workspace/bin/ff* /usr/bin/

RUN ldd /usr/bin/ffmpeg && \
    ldd /usr/bin/ffprobe && \
    ldd /usr/bin/ffplay

# =============================================================================
# Stage: server
# =============================================================================
FROM base AS server
COPY ./dist/transcoderd-server /app/transcoderd-server
ENTRYPOINT ["/app/transcoderd-server"]

# =============================================================================
# Stage: worker
# =============================================================================
FROM ${PGS_IMAGE} AS pgs-bins

FROM base AS worker
WORKDIR /app
COPY --from=pgs-bins /src/tessdata /app/tessdata
COPY --from=pgs-bins /src/PgsToSrt/out /app
RUN apt-get update && \
    apt-get install -y libtesseract-dev && \
    apt-get clean && rm -rf /var/lib/apt/lists/*
RUN wget https://dot.net/v1/dotnet-install.sh -O dotnet-install.sh && \
    chmod +x dotnet-install.sh && \
    ./dotnet-install.sh --channel 8.0 --runtime dotnet --install-dir /usr/share/dotnet && \
    ln -s /usr/share/dotnet/dotnet /usr/bin/dotnet && \
    rm dotnet-install.sh

COPY ./dist/transcoderd-worker /app/transcoderd-worker
ENTRYPOINT ["/app/transcoderd-worker"]

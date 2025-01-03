ARG BASE_IMAGE=ubuntu:22.04@sha256:77906da86b60585ce12215807090eb327e7386c8fafb5402369e421f44eff17e
FROM ${BASE_IMAGE} AS builder-ffmpeg

ARG FFMPEG_BUILD_SCRIPT_VERSION=1.48

ENV DEBIAN_FRONTEND=noninteractive

RUN apt-get update \
    && apt-get -y --no-install-recommends install \
        build-essential \
        curl \
        ca-certificates \
        libva-dev \
        python3 \
        python-is-python3 \
        ninja-build \
        meson \
    && apt-get clean; rm -rf /var/lib/apt/lists/* /tmp/* /var/tmp/* /usr/share/doc/* \
    && update-ca-certificates

WORKDIR /app

# ADD doesn't cache when used from URL
RUN curl -sLO \
    https://raw.githubusercontent.com/markus-perl/ffmpeg-build-script/v${FFMPEG_BUILD_SCRIPT_VERSION}/build-ffmpeg && \
    chmod 755 ./build-ffmpeg && \
    SKIPINSTALL=yes ./build-ffmpeg \
        --build \
        --enable-gpl-and-non-free && \
    rm -rf packages && \
    find workspace -mindepth 1 -maxdepth 1 -type d ! -name 'bin' -exec rm -rf {} \; && \
    find workspace/bin -mindepth 1 -maxdepth 1 -type f ! -name 'ff*' -exec rm -f {} \;

FROM debian:trixie-20241223-slim  AS base
RUN apt-get update \
    && apt-get install -y \
        ca-certificates \
        mkvtoolnix \
        libva-drm2 \
        wget \
    && rm -rf /var/lib/apt/lists/*

COPY --from=builder-ffmpeg /app/workspace/bin/ff* /usr/bin/

# Check shared library
RUN ldd /usr/bin/ffmpeg && \
    ldd /usr/bin/ffprobe && \
    ldd /usr/bin/ffplay

FROM base AS server
COPY ./dist/transcoderd-server /app/transcoderd-server
ENTRYPOINT ["/app/transcoderd-server"]

FROM mcr.microsoft.com/dotnet/sdk:6.0 AS builder-pgs
WORKDIR /src
ADD https://github.com/Tentacule/PgsToSrt/archive/refs/heads/master.zip pgstosrt.zip
ADD https://github.com/tesseract-ocr/tessdata/archive/refs/heads/main.zip tessdata.zip
RUN apt-get -y update && \
  apt-get -y upgrade && \
  apt-get -y install \
    automake \
    ca-certificates \
    g++ \
    libtool \
    libtesseract4 \
    make \
    pkg-config \
    wget \
    unzip \
    libc6-dev

RUN unzip tessdata.zip && \
    rm tessdata.zip && \
    mv tessdata-main tessdata

RUN unzip pgstosrt.zip && \
    rm pgstosrt.zip && \
    cd PgsToSrt-master/src && \
    dotnet restore  && \
    dotnet publish -c Release -f net6.0 -o /src/PgsToSrt/out

FROM base AS worker
WORKDIR /app
COPY --from=builder-pgs /src/tessdata /app/tessdata
COPY --from=builder-pgs /src/PgsToSrt/out /app
RUN wget https://packages.microsoft.com/config/ubuntu/22.04/packages-microsoft-prod.deb && \
    dpkg -i packages-microsoft-prod.deb && \
    apt-get update && \
    apt-get install -y dotnet-runtime-6.0 libtesseract-dev && \
    apt-get clean && \
    rm -rf /var/lib/apt/lists/* && \
    rm -f packages-microsoft-prod.deb


COPY ./dist/transcoderd-worker /app/transcoderd-worker

ENTRYPOINT ["/app/transcoderd-worker"]


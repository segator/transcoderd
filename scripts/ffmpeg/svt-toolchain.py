#!/usr/bin/env python3
"""Relink the pinned production toolchain with one checksum-pinned SVT version.

Runs inside the existing Linux FFmpeg builder. Both benchmark variants use this
same procedure and retain every other installed dependency and configure flag.
"""

import argparse
import hashlib
import json
import os
from pathlib import Path
import shlex
import subprocess
import tarfile
import urllib.request

HASHES = {
    "4.2.0": "c7b13c4a84bd3751aa35fcc72be13e6875467e7c2216879251a486e5b1e4e740",
    "4.3.0": "95ce8fc2381ba707e65ac8b9b2e165915bfbe9cd4d2f7b0f58d64ce31bd98f6d",
}
FFMPEG_HASH = "6e374ed621e48faa40639307dff48ba6fe574a509977956d2cce9669b7cc27e9"


def override(text, version):
    old = f'VER_SVTAV1=("4.2.0" "{HASHES["4.2.0"]}")'
    if text.count(old) != 1:
        raise ValueError("Expected exactly one pinned SVT-AV1 4.2.0 entry")
    return text.replace(old, f'VER_SVTAV1=("{version}" "{HASHES[version]}")')


def fetch(url, checksum, destination):
    archive = destination.with_suffix(".tar.gz")
    urllib.request.urlretrieve(url, archive)
    if hashlib.file_digest(archive.open("rb"), "sha256").hexdigest() != checksum:
        raise ValueError(f"Checksum mismatch: {url}")
    destination.mkdir()
    with tarfile.open(archive) as data:
        # Extraction uses the data filter, followed by the archive's single root.
        data.extractall(destination, filter="data")
    archive.unlink()
    return next(destination.iterdir())


def main():
    parser = argparse.ArgumentParser(description=__doc__)
    parser.add_argument("version", choices=HASHES)
    args = parser.parse_args()
    root = Path("/app/ffmpeg-build-script")
    workspace = root / "workspace"
    versions = root / "src/10-versions.sh"
    versions.write_text(override(versions.read_text(), args.version))
    banner = subprocess.check_output([str(workspace / "bin/ffmpeg"), "-version"], text=True)
    config = next(line.removeprefix("configuration: ") for line in banner.splitlines()
                  if line.startswith("configuration: "))
    env = dict(os.environ)
    env["PATH"] = f"{workspace}/bin:/usr/local/cuda/bin:" + env["PATH"]
    # Match the build script's workspace-first search while retaining system
    # SDKs such as libva. Its private pkg-config has no system default path.
    multiarch = subprocess.check_output(["gcc", "-print-multiarch"], text=True).strip()
    env["PKG_CONFIG_PATH"] = ":".join([
        str(workspace / "lib/pkgconfig"),
        f"/usr/local/lib/{multiarch}/pkgconfig", f"/usr/lib/{multiarch}/pkgconfig",
        "/usr/local/lib/pkgconfig", "/usr/local/share/pkgconfig",
        "/usr/lib/pkgconfig", "/usr/share/pkgconfig", "/usr/lib64/pkgconfig",
    ])
    jobs = os.environ.get("SVT_BUILD_JOBS", "8")

    def run(command, cwd=None):
        subprocess.run(command, cwd=cwd, env=env, check=True)

    work = Path("/svt-rebuild")
    work.mkdir()
    svt = fetch(f"https://gitlab.com/AOMediaCodec/SVT-AV1/-/archive/v{args.version}/SVT-AV1-v{args.version}.tar.gz",
                HASHES[args.version], work / "svt")
    build = svt / "Build/linux"
    run(["cmake", f"-DCMAKE_INSTALL_PREFIX={workspace}", "-DENABLE_SHARED=off",
         "-DBUILD_SHARED_LIBS=OFF", "../..", "-GUnix Makefiles", "-DCMAKE_BUILD_TYPE=Release"], build)
    run(["make", "-j", jobs], build)
    run(["make", "install"], build)
    (workspace / "lib/pkgconfig/SvtAv1Enc.pc").write_bytes((build / "SvtAv1Enc.pc").read_bytes())
    ffmpeg = fetch("https://github.com/FFmpeg/FFmpeg/archive/refs/tags/n9.0.2.tar.gz",
                   FFMPEG_HASH, work / "ffmpeg")
    run(["./configure", *shlex.split(config)], ffmpeg)
    run(["make", "-j", jobs], ffmpeg)
    run(["make", "install"], ffmpeg)
    manifest = {
        "svt_version": args.version, "svt_sha256": HASHES[args.version],
        "ffmpeg_version": "9.0.2", "ffmpeg_sha256": FFMPEG_HASH,
        "configure": shlex.split(config),
        "compiler": subprocess.check_output(["gcc", "--version"], text=True),
        "svt_cmake": ["Release", "ENABLE_SHARED=off", "BUILD_SHARED_LIBS=OFF"],
    }
    (workspace / "svt-toolchain.json").write_text(json.dumps(manifest, indent=2) + "\n")


if __name__ == "__main__":
    main()

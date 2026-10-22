#!/usr/bin/env python3
"""Compare SVT presets 3–6 against the worker NVENC profile at similar p5 VMAF.

Offline, sequential video-only lab; it does not change worker behavior. The
manifest is a JSON array of title/source/reference/start/initial_crf objects.
References must be lossless decodes of the exact source interval. Source files
are read only. Every measured command and every scored frame is retained.
"""

import argparse
import json
import math
import statistics
from pathlib import Path

import av1_lab as lab

REFERENCE_FRAMES = {}


def measure(args, scene, codec, quality, directory):
    name = f"{codec}-q{quality}"
    record = directory / f"{name}.json"
    encoded = directory / f"{name}.mkv"
    command = [args.ffmpeg, "-hide_banner", "-nostdin", "-y", "-fflags", "+genpts",
               "-threads", "0", "-analyzeduration", "2147483647", "-probesize", "2147483647",
               "-ss", str(scene["start"]), "-i", scene["source"], "-t", str(args.duration),
               "-map", "0:v:0", "-an", "-sn", "-dn", "-fps_mode", "passthrough",
               "-filter:v", "scale='min(1920,iw)':-1:force_original_aspect_ratio=decrease"]
    if codec == "nvenc-p7":
        command += ["-pix_fmt", "p010le", "-c:v", "av1_nvenc", "-preset", "p7",
                    "-tune", "uhq", "-rc", "vbr", "-cq", str(quality), "-b:v", "0",
                    "-multipass", "fullres", "-bf", "4", "-b_ref_mode", "middle",
                    "-split_encode_mode", "disabled", "-qmin", "0", "-qmax", "100"]
    else:
        command += ["-pix_fmt", "yuv420p10le", "-c:v", "libsvtav1", "-preset",
                    codec.removeprefix("svt-p"), "-crf", str(quality)]
    command.append(str(encoded))
    if record.exists():
        cached = json.loads(record.read_text())
        if cached["command"] != command or cached["reference"] != scene["reference"]:
            raise RuntimeError(f"Cached measurement differs from manifest/settings: {record}; use a new output directory")
        REFERENCE_FRAMES[scene["reference"]] = cached["reference_frames"]
        return cached
    print(f"Encoding {scene['title']} {name}", flush=True)
    seconds = lab.ffmpeg_run(command, directory / f"{name}.log")
    if scene["reference"] not in REFERENCE_FRAMES:
        REFERENCE_FRAMES[scene["reference"]] = lab.frames(scene["reference"], args.ffprobe)
    expected = REFERENCE_FRAMES[scene["reference"]]
    if lab.frames(encoded, args.ffprobe) != expected:
        raise RuntimeError(f"Frame count mismatch: {encoded}")
    scores_path = directory / f"{name}-vmaf.json"
    filters = ("[0:v]setpts=PTS-STARTPTS[d];[1:v]setpts=PTS-STARTPTS[r];"
               f"[d][r]libvmaf=n_threads=8:n_subsample=1:log_fmt=json:log_path={scores_path}")
    lab.ffmpeg_run([args.ffmpeg, "-hide_banner", "-nostdin", "-i", str(encoded),
                    "-i", scene["reference"], "-lavfi", filters, "-f", "null", "-"],
                   directory / f"{name}-vmaf.log")
    scores = json.loads(scores_path.read_text())
    values = sorted(frame["metrics"]["vmaf"] for frame in scores["frames"])
    if len(values) != expected:
        raise RuntimeError(f"VMAF frame count mismatch: {encoded}")
    row = dict(scene, profile=codec, quality=quality, command=command,
               duration=args.duration, encode_seconds=seconds,
               video_bytes=lab.video_packet_bytes(encoded, args.ffprobe),
               vmaf_p5=values[math.ceil(len(values) * .05) - 1],
               vmaf_mean=scores["pooled_metrics"]["vmaf"]["mean"],
               reference_frames=expected, scored_frames=len(values))
    record.write_text(json.dumps(row, indent=2) + "\n")
    print(f"  {row['video_bytes']/1e6:.3f} MB, p5 {row['vmaf_p5']:.3f}, {seconds:.2f}s", flush=True)
    return row


def main():
    parser = argparse.ArgumentParser(description=__doc__)
    parser.add_argument("--manifest", type=Path, required=True)
    parser.add_argument("--output", type=Path, required=True)
    parser.add_argument("--ffmpeg", required=True)
    parser.add_argument("--ffprobe", default="ffprobe")
    parser.add_argument("--duration", type=int, default=60)
    parser.add_argument("--repeats", type=int, default=3)
    parser.add_argument("--p5-tolerance", type=float, default=.3)
    args = parser.parse_args()
    if args.repeats < 1:
        parser.error("--repeats must be positive")
    if not 0 < args.p5_tolerance <= 1:
        parser.error("--p5-tolerance must be greater than zero and at most one")
    args.output = args.output.resolve()
    args.output.mkdir(parents=True, exist_ok=True)
    summaries = []
    for index, scene in enumerate(json.loads(args.manifest.read_text())):
        directory = args.output / f"scene-{index+1}"
        directory.mkdir(exist_ok=True)
        gpu = measure(args, scene, "nvenc-p7", 34, directory)
        summaries.append(dict(gpu, target_p5=gpu["vmaf_p5"]))
        floor = max(91, gpu["vmaf_p5"] - args.p5_tolerance)
        initial_crf = int(scene["initial_crf"])
        for preset in (3, 4, 5, 6):
            quality = initial_crf
            tried = {}
            for _ in range(6):
                point = measure(args, scene, f"svt-p{preset}", quality, directory)
                tried[quality] = point
                delta = point["vmaf_p5"] - gpu["vmaf_p5"]
                if point["vmaf_p5"] >= floor and delta <= args.p5_tolerance:
                    break
                passing = [q for q, r in tried.items() if r["vmaf_p5"] >= floor]
                failing = [q for q, r in tried.items() if r["vmaf_p5"] < floor]
                if passing and failing:
                    low, high = max(passing), min(failing)
                    if high - low <= 1:
                        break
                    quality = (low + high) // 2
                else:
                    step = max(1, min(6, round(abs(delta) / .4)))
                    quality = max(1, min(63, quality + (step if delta > 0 else -step)))
                if quality in tried:
                    break
            passing = [r for r in tried.values() if r["vmaf_p5"] >= floor]
            if not passing:
                raise RuntimeError(f"No quality match: {scene['title']} preset {preset}; expand the CRF search")
            best = min(passing, key=lambda r: r["video_bytes"])
            initial_crf = best["quality"]
            summaries.append(dict(best, target_p5=gpu["vmaf_p5"], quality_floor=floor,
                                  p5_tolerance=args.p5_tolerance))
            (args.output / "selected.json").write_text(json.dumps(summaries, indent=2) + "\n")

    # Repeat only the selected points, with no scoring or other encode running.
    # Cache the repetitions separately so an interrupted run can resume.
    for row in summaries:
        directory = Path(row["command"][-1]).parent
        timing_path = directory / f"{row['profile']}-q{row['quality']}-timings.json"
        timings = json.loads(timing_path.read_text()) if timing_path.exists() else [row["encode_seconds"]]
        while len(timings) < args.repeats:
            timings.append(lab.ffmpeg_run(row["command"], directory / f"{row['profile']}-repeat{len(timings)+1}.log"))
            timing_path.write_text(json.dumps(timings) + "\n")
        if lab.video_packet_bytes(row["command"][-1], args.ffprobe) != row["video_bytes"]:
            raise RuntimeError("Repeated encode changed video bytes; inspect before reusing quality scores")
        row["encode_seconds_runs"] = timings
        row["encode_seconds_median"] = statistics.median(timings)
        print(f"Selected {row['title']} {row['profile']} Q{row['quality']}: "
              f"{row['encode_seconds_median']:.2f}s median of {len(timings)}", flush=True)
    (args.output / "selected.json").write_text(json.dumps(summaries, indent=2) + "\n")


if __name__ == "__main__":
    main()

#!/usr/bin/env python3
"""Offline, sequential SVT version comparison using the current CPU recipe.

Manifest: [{"title": "scene", "reference": "/absolute/lossless.mkv"}].
All reference frames are encoded. Use a new output directory when changing any
input, binary, build manifest, metric model, or command. No sources are modified.
"""

import argparse
import csv
from fractions import Fraction
import hashlib
import json
import math
import os
from pathlib import Path
import platform
import re
import statistics
import subprocess
import time

import av1_lab as lab

MODEL = "vmaf_v0.6.1"
SCALE = "scale='min(1920,iw)':-1:force_original_aspect_ratio=decrease"


def sha256(path):
    with Path(path).open("rb") as data:
        return hashlib.file_digest(data, "sha256").hexdigest()


def save(path, value):
    temporary = path.with_suffix(path.suffix + ".partial")
    temporary.write_text(json.dumps(value, indent=2, allow_nan=False) + "\n")
    temporary.replace(path)


def run(command, log):
    started = time.monotonic()
    with log.open("w") as output:
        result = subprocess.run(command, stdout=output, stderr=subprocess.STDOUT)
    if result.returncode:
        raise RuntimeError(f"Command failed ({result.returncode}); see {log}")
    return time.monotonic() - started


def fingerprint(path):
    path = Path(path).resolve(strict=True)
    record = {"path": str(path), "sha256": sha256(path)}
    manifest = path.parent.parent / "toolchain.json"
    if manifest.is_file():
        record["toolchain"] = json.loads(manifest.read_text())
        record["toolchain_sha256"] = sha256(manifest)
    return record


def check_versions(baseline, candidate):
    if baseline["sha256"] == candidate["sha256"]:
        raise ValueError("Baseline and candidate are the same binary")


def cache_key(tools, scenes, crfs, repeats, contended):
    payload = {"tools": tools, "scenes": scenes, "crfs": crfs, "repeats": repeats,
               "timing_contended": contended, "model": MODEL, "scale": SCALE,
               "platform": platform.platform(), "affinity": sorted(os.sched_getaffinity(0)),
               "cpu": next(line for line in Path("/proc/cpuinfo").read_text().splitlines()
                           if line.startswith("model name")),
               "runner_sha256": sha256(__file__), "lab_sha256": sha256(lab.__file__)}
    return hashlib.sha256(json.dumps(payload, sort_keys=True).encode()).hexdigest()


def check_cache(path, key):
    if path.exists() and json.loads(path.read_text())["key"] != key:
        raise ValueError("Cached inputs/tools/settings differ; use a new output directory")


def encode_command(binary, reference, output, crf):
    return [binary, "-hide_banner", "-nostdin", "-nostats", "-progress", "pipe:1", "-y", "-fflags", "+genpts",
            "-threads", "0", "-analyzeduration", "2147483647", "-probesize", "2147483647",
            "-i", str(reference), "-max_muxing_queue_size", "9999", "-map", "0:v:0", "-an", "-sn", "-dn",
            "-avoid_negative_ts", "make_zero", "-copyts", "-map_chapters", "-1",
            "-flags", "+global_header", "-filter:v", SCALE, "-pix_fmt", "yuv420p10le",
            "-c:v", "libsvtav1", "-preset", "4", "-crf", str(crf),
            "-fps_mode", "passthrough", str(output)]


def timeline(probe, path):
    data = json.loads(lab.command([probe, "-v", "error", "-select_streams", "v:0",
                                  "-show_frames", "-show_entries",
                                  "frame=best_effort_timestamp_time", "-of", "json", str(path)]).stdout)
    times = [float(frame["best_effort_timestamp_time"]) for frame in data["frames"]]
    if not times or any(not math.isfinite(t) for t in times):
        raise ValueError(f"Missing or non-finite frame timestamps: {path}")
    if any(b <= a for a, b in zip(times, times[1:])):
        raise ValueError(f"Duplicate or reversed frame timestamps: {path}")
    return [round(t - times[0], 6) for t in times]


def check_timeline(reference, output):
    if len(reference) != len(output) or any(abs(a - b) > .0011 for a, b in zip(reference, output)):
        raise ValueError("Reference/output frame counts or normalized timelines differ")


def quality_summary(data, expected, fps):
    frames = data["frames"]
    if len(frames) != expected or [f["frameNum"] for f in frames] != list(range(expected)):
        raise ValueError("Scored frame count/indices differ from reference")
    values = [f["metrics"]["vmaf"] for f in frames]
    if not values or any(not math.isfinite(v) for v in values):
        raise ValueError("Missing or non-finite VMAF")
    ordered = sorted(values)
    window = min(expected, max(1, round(fps)))
    means = [sum(values[i:i+window]) / window for i in range(expected - window + 1)]
    return {"vmaf_mean": statistics.mean(values),
            "vmaf_p5": ordered[math.ceil(expected * .05) - 1],
            "vmaf_p1": ordered[math.ceil(expected * .01) - 1],
            "vmaf_min": min(values), "worst_frame": values.index(min(values)),
            "worst_1s_mean": min(means), "worst_1s_start": means.index(min(means)),
            "scored_frames": expected}


def decoded_hash(ffmpeg, path, directory, name):
    target = directory / f"{name}-framemd5.txt"
    run([ffmpeg, "-v", "error", "-xerror", "-i", str(path), "-map", "0:v:0",
         "-pix_fmt", "yuv420p10le", "-f", "framemd5", "-y", str(target)],
        directory / f"{name}-decode.log")
    hashes = [line.split(",")[-1].strip() for line in target.read_text().splitlines()
              if line and not line.startswith("#")]
    return hashlib.sha256("\n".join(hashes).encode()).hexdigest(), len(hashes)


def score(args, reference, encoded, directory, name, expected, fps):
    vmaf_path = directory / f"{name}-vmaf.json"
    # All local metric paths are simple relative names, avoiding filter escaping
    # problems for the user's input/output paths (including spaces and colons).
    graph = ("[0:v]format=yuv420p,setpts=PTS-STARTPTS[d];"
             "[1:v]format=yuv420p,setpts=PTS-STARTPTS[r];"
             f"[d][r]libvmaf=model=version={MODEL}:n_threads=8:n_subsample=1:"
             f"log_fmt=json:log_path={vmaf_path.name}")
    commands = []
    for metric in ("vmaf", "ssim", "psnr"):
        if metric == "vmaf":
            filters = graph
        else:
            filters = graph[:graph.index("[d][r]")] + f"[d][r]{metric}=stats_file={name}-{metric}.txt"
        command = [args.metric_ffmpeg, "-hide_banner", "-nostdin", "-i", str(encoded),
                   "-i", reference, "-filter_complex", filters, "-f", "null", "-"]
        commands.append(command)
        with (directory / f"{name}-{metric}.log").open("w") as log:
            subprocess.run(command, cwd=directory, stdout=log, stderr=subprocess.STDOUT, check=True)
    result = quality_summary(json.loads(vmaf_path.read_text()), expected, fps)
    for metric in ("ssim", "psnr"):
        lines = (directory / f"{name}-{metric}.txt").read_text().splitlines()
        if len(lines) != expected or [int(re.search(r"\bn:(\d+)", s)[1]) for s in lines] != list(range(1, expected + 1)):
            raise ValueError(f"{metric} frame count/indices differ")
        field = "All" if metric == "ssim" else "mse_avg"
        values = [float(re.search(rf"\b{field}:([^\s]+)", s)[1]) for s in lines]
        if any(not math.isfinite(v) for v in values):
            raise ValueError(f"Non-finite {metric}")
        value = statistics.mean(values)
        # PSNR from aggregate per-frame MSE of the explicit 8-bit metric format.
        result[metric] = value if metric == "ssim" else (10 * math.log10(255 ** 2 / value) if value else None)
    return result, commands


def utilization():
    lines = Path("/proc/stat").read_text().splitlines()[0].split()[1:9]
    values = [int(value) for value in lines]
    return {"total": sum(values), "idle": values[3] + values[4], "loadavg": os.getloadavg()}


def compare(args, scene, directory, crf, rows):
    pair = [row for row in rows if row["scene"] == scene["title"] and row["crf"] == crf]
    baseline, candidate = pair
    arrays = []
    for label in ("baseline", "candidate"):
        data = json.loads((directory / f"{label}-crf{crf}-run1-vmaf.json").read_text())
        arrays.append([frame["metrics"]["vmaf"] for frame in data["frames"]])
    deltas = [b - a for a, b in zip(*arrays)]
    worst = deltas.index(min(deltas))
    summary = {"scene": scene["title"], "crf": crf,
               "video_bytes_change_percent": 100 * (candidate["video_bytes"] / baseline["video_bytes"] - 1),
               "observed_time_reduction_percent": 100 * (1 - candidate["seconds_median"] / baseline["seconds_median"]),
               "timing_contended": args.timing_contended,
               "worst_frame_delta": min(deltas), "worst_delta_frame": worst}
    for field in ("vmaf_mean", "vmaf_p5", "vmaf_p1", "ssim", "psnr"):
        summary[field + "_delta"] = (candidate[field] - baseline[field]
                                     if candidate[field] is not None and baseline[field] is not None else None)
    inputs = [scene["reference"], str(directory / f"baseline-crf{crf}-run1.mkv"),
              str(directory / f"candidate-crf{crf}-run1.mkv")]
    for name, index in (("representative", len(deltas) // 2), ("worst-delta", worst)):
        panels = []
        for i, label in enumerate(("Source", "SVT 4.2.0", "SVT 4.3.0")):
            # Native-size center crops; no enlargement or smoothing that could
            # conceal texture differences. drawtext uses the metric build font.
            panels.append(f"[{i}:v]select=eq(n\\,{index}),crop=w='min(iw,640)':h='min(ih,360)',setpts=PTS-STARTPTS,"
                          f"pad=iw:ih+30:0:30:color=black,drawtext=text='{label} frame {index}':"
                          f"fontcolor=white:fontsize=20:x=8:y=4[p{i}]")
        graph = ";".join(panels) + ";[p0][p1][p2]hstack=inputs=3"
        command = [args.metric_ffmpeg, "-v", "error", "-nostdin"]
        for path in inputs:
            command += ["-i", path]
        command += ["-filter_complex", graph, "-frames:v", "1", "-y",
                    str(directory / f"crf{crf}-{name}.png")]
        run(command, directory / f"crf{crf}-{name}-visual.log")
    return summary


def main():
    parser = argparse.ArgumentParser(description=__doc__)
    parser.add_argument("--baseline", required=True)
    parser.add_argument("--candidate", required=True)
    parser.add_argument("--metric-ffmpeg", required=True)
    parser.add_argument("--ffprobe", required=True)
    parser.add_argument("--manifest", type=Path, required=True)
    parser.add_argument("--output", type=Path, required=True)
    parser.add_argument("--repeats", type=int, default=3)
    parser.add_argument("--crfs", default="25")
    parser.add_argument("--timing-contended", action="store_true",
                        help="Retain results but exclude speed claims on a busy host")
    args = parser.parse_args()
    crfs = [int(v) for v in args.crfs.split(",")]
    if args.repeats < 3 or not crfs or any(v < 1 or v > 63 for v in crfs):
        parser.error("Use at least three repetitions and CRFs 1..63")
    for field in ("baseline", "candidate", "metric_ffmpeg", "ffprobe"):
        setattr(args, field, str(Path(getattr(args, field)).resolve(strict=True)))
    tools = {key: fingerprint(getattr(args, key)) for key in ("baseline", "candidate", "metric_ffmpeg", "ffprobe")}
    check_versions(tools["baseline"], tools["candidate"])
    scenes = json.loads(args.manifest.read_text())
    for scene in scenes:
        scene["reference"] = str(Path(scene["reference"]).resolve(strict=True))
        scene["sha256"] = sha256(scene["reference"])
    args.output = args.output.resolve()
    args.output.mkdir(parents=True, exist_ok=True)
    key = cache_key(tools, scenes, crfs, args.repeats, args.timing_contended)
    identity_path = args.output / "identity.json"
    check_cache(identity_path, key)
    save(identity_path, {"key": key, "tools": tools, "scenes": scenes, "model": MODEL,
                         "platform": platform.platform(), "cpu": Path("/proc/cpuinfo").read_text(),
                         "memory": Path("/proc/meminfo").read_text(), "affinity": sorted(os.sched_getaffinity(0)),
                         "timing_contended": args.timing_contended})
    rows, comparisons = [], []
    for index, scene in enumerate(scenes):
        directory = args.output / f"scene-{index+1}"
        directory.mkdir(exist_ok=True)
        info = lab.probe(scene["reference"], args.ffprobe)
        stream = next(s for s in info["streams"] if s["codec_type"] == "video")
        if stream["codec_name"] not in ("ffv1", "rawvideo", "huffyuv"):
            raise ValueError("Use a decoded lossless reference (FFV1, rawvideo or HuffYUV)")
        if stream.get("color_transfer") in ("smpte2084", "arib-std-b67"):
            raise ValueError("HDR is outside this SDR metric comparison")
        if stream["width"] > 1920:
            raise ValueError("Reference must already use the worker's output dimensions")
        times = timeline(args.ffprobe, scene["reference"])
        expected, fps = len(times), float(Fraction(stream["avg_frame_rate"]))
        for crf in crfs:
            # Warm up each build before timed runs. No metrics or other encodes
            # run during timings. Timed outputs are retained and checked later.
            for label in ("baseline", "candidate"):
                run(encode_command(getattr(args, label), scene["reference"],
                                   directory / f"{label}-warmup.mkv", crf),
                    directory / f"{label}-warmup.log")
            records = {label: [] for label in ("baseline", "candidate")}
            for repeat in range(args.repeats):
                for label in (("baseline", "candidate") if repeat % 2 == 0 else ("candidate", "baseline")):
                    name = f"{label}-crf{crf}-run{repeat+1}"
                    output = directory / f"{name}.mkv"
                    record_path = directory / f"{name}.json"
                    command = encode_command(getattr(args, label), scene["reference"], output, crf)
                    if record_path.exists():
                        record = json.loads(record_path.read_text())
                        if record["command"] != command or record["output_sha256"] != sha256(output):
                            raise ValueError(f"Cached command/output changed: {name}")
                    else:
                        before = utilization()
                        print(f"Encoding {scene['title']} {name}", flush=True)
                        seconds = run(command, directory / f"{name}.log")
                        after = utilization()
                        log = (directory / f"{name}.log").read_text()
                        version = "4.2.0" if label == "baseline" else "4.3.0"
                        if not re.search(rf"SVT-AV1 Encoder Lib v{re.escape(version)}(?:\s|$)", log):
                            raise ValueError(f"Unexpected actual SVT version in {name}")
                        record = {"command": command, "seconds": seconds, "before": before, "after": after,
                                  "output_sha256": sha256(output)}
                        save(record_path, record)
                    records[label].append(record)
            for label in ("baseline", "candidate"):
                pixels = None
                quality = None
                for repeat, record in enumerate(records[label]):
                    name = f"{label}-crf{crf}-run{repeat+1}"
                    encoded = directory / f"{name}.mkv"
                    check_timeline(times, timeline(args.ffprobe, encoded))
                    output_stream = next(s for s in lab.probe(encoded, args.ffprobe)["streams"] if s["codec_type"] == "video")
                    if (output_stream["codec_name"], output_stream["pix_fmt"], output_stream["width"],
                        output_stream["height"], Fraction(output_stream["avg_frame_rate"])) != (
                            "av1", "yuv420p10le", stream["width"], stream["height"], Fraction(stream["avg_frame_rate"])):
                        raise ValueError(f"Unexpected output format: {name}")
                    decoded, count = decoded_hash(args.metric_ffmpeg, encoded, directory, name)
                    if count != expected:
                        raise ValueError(f"Strict decode frame mismatch: {name}")
                    if pixels is not None and pixels != decoded:
                        raise ValueError(f"Repeated encode changed decoded frames: {name}; score separately")
                    pixels = decoded
                    if quality is None:
                        quality, commands = score(args, scene["reference"], encoded, directory, name, expected, fps)
                        save(directory / f"{label}-crf{crf}-metrics.json", {"quality": quality, "commands": commands})
                seconds = [r["seconds"] for r in records[label]]
                output = directory / f"{label}-crf{crf}-run1.mkv"
                row = {"scene": scene["title"], "version": "4.2.0" if label == "baseline" else "4.3.0",
                       "crf": crf, "frames": expected, "seconds_runs": seconds,
                       "seconds_median": statistics.median(seconds), "seconds_stdev": statistics.stdev(seconds),
                       "fps": expected / statistics.median(seconds), "container_bytes": output.stat().st_size,
                       "video_bytes": lab.video_packet_bytes(output, args.ffprobe), "decoded_sha256": pixels,
                       "timing_contended": args.timing_contended, **quality}
                rows.append(row)
                save(args.output / "results.json", rows)
                print(f"Measured {row['scene']} {row['version']} CRF{crf}: "
                      f"{row['seconds_median']:.3f}s, {row['video_bytes']} bytes, "
                      f"VMAF {row['vmaf_mean']:.3f}/p5 {row['vmaf_p5']:.3f}", flush=True)
            comparisons.append(compare(args, scene, directory, crf, rows))
            save(args.output / "comparisons.json", comparisons)
    with (args.output / "results.csv").open("w") as output:
        writer = csv.DictWriter(output, fieldnames=list(rows[0]))
        writer.writeheader()
        writer.writerows(rows)


if __name__ == "__main__":
    main()

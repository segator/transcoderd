#!/usr/bin/env python3
"""Read-only source inventory and reproducible AV1 rate/quality/time lab."""

import argparse
from concurrent.futures import ThreadPoolExecutor
import csv
import hashlib
import json
import math
import os
from pathlib import Path
import platform
import re
import statistics
import subprocess
import sys
import time


VIDEO_EXTENSIONS = {".avi", ".m2ts", ".m4v", ".mkv", ".mov", ".mp4", ".ts", ".webm"}
PROFILES = {
    "nvenc-p5": ("av1_nvenc", ["-preset", "p5", "-tune", "hq"]),
    "nvenc-p7": ("av1_nvenc", ["-preset", "p7", "-tune", "uhq"]),
    "svt-p4": ("libsvtav1", ["-preset", "4"]),
    "svt-p6": ("libsvtav1", ["-preset", "6"]),
}
DEFAULT_QUALITIES = {"nvenc": [18, 22, 26, 30, 36, 42], "svt": [10, 16, 22, 28, 36, 44]}


def command(argv):
    return subprocess.run(argv, check=True, stdout=subprocess.PIPE, stderr=subprocess.PIPE, text=True)


def probe(path, ffprobe):
    output = command([ffprobe, "-v", "error", "-show_format", "-show_streams", "-of", "json", str(path)])
    return json.loads(output.stdout)


def video_packet_bytes(path, ffprobe):
    """Sum actual video packet payload bytes without loading packets into memory."""
    process = subprocess.Popen(
        [ffprobe, "-v", "error", "-select_streams", "v:0", "-show_entries", "packet=size",
         "-of", "csv=p=0", str(path)], stdout=subprocess.PIPE, stderr=subprocess.PIPE, text=True
    )
    total = 0
    try:
        for line in process.stdout:
            size = line.strip().split(",")[0]
            if size.isdecimal():
                total += int(size)
        stderr = process.stderr.read()
        if process.wait() != 0:
            raise RuntimeError(f"ffprobe packet scan failed for {path}: {stderr}")
    finally:
        if process.poll() is None:
            process.kill()
            process.wait()
    return total


def media_paths(inputs):
    found = set()
    for item in inputs:
        path = Path(item).expanduser().resolve()
        if path.is_file():
            found.add(path)
        elif path.is_dir():
            found.update(p for p in path.rglob("*") if p.is_file() and p.suffix.lower() in VIDEO_EXTENSIONS)
        else:
            raise FileNotFoundError(path)
    return sorted(found)


def metadata(path, ffprobe, packets=False):
    data = probe(path, ffprobe)
    video = next((s for s in data["streams"] if s["codec_type"] == "video"), None)
    if video is None:
        raise ValueError(f"No video stream: {path}")
    duration = float(video.get("duration") or data["format"].get("duration") or 0)
    if duration <= 0:
        raise ValueError(f"Missing duration: {path}")
    pixel_format = video.get("pix_fmt", "unknown")
    bits = video.get("bits_per_raw_sample") or video.get("bits_per_sample")
    if not bits:
        match = re.search(r"(?:p0|p|yuv\d+p)(10|12|16)", pixel_format)
        bits = match.group(1) if match else "8" if pixel_format != "unknown" else "0"
    transfer = video.get("color_transfer", "unknown")
    hdr = transfer in {"smpte2084", "arib-std-b67"}
    row = {
        "path": str(path), "file_bytes": path.stat().st_size, "mtime_ns": path.stat().st_mtime_ns,
        "codec": video["codec_name"], "width": video["width"], "height": video["height"],
        "pixel_format": pixel_format, "bit_depth": int(bits) if str(bits).isdigit() else 0,
        "color_transfer": transfer, "hdr": hdr, "duration_seconds": duration,
        "audio_streams": sum(s["codec_type"] == "audio" for s in data["streams"]),
        "subtitle_streams": sum(s["codec_type"] == "subtitle" for s in data["streams"]),
        "video_packet_bytes": video_packet_bytes(path, ffprobe) if packets else None,
    }
    return row


def category(row):
    height = row["height"]
    resolution = "4k+" if height > 1440 else "1440p" if height > 1080 else "1080p" if height > 720 else "720p" if height > 480 else "sd"
    return f'{row["codec"]}/{resolution}/{row["bit_depth"]}bit/{"hdr" if row["hdr"] else "sdr"}'


def windows(duration, length, count):
    if duration <= length:
        return [(0.0, duration)]
    fractions = [0.15, 0.50, 0.85]
    if count > len(fractions):
        fractions = [(i + 1) / (count + 1) for i in range(count)]
    return [(round((duration - length) * f, 3), length) for f in fractions[:count]]


def append_jsonl(path, row):
    with path.open("a", encoding="utf-8") as handle:
        handle.write(json.dumps(row, sort_keys=True) + "\n")


def jsonl(path):
    if not path.exists():
        return []
    with path.open(encoding="utf-8") as handle:
        return [json.loads(line) for line in handle if line.strip()]


def inventory(args):
    target = Path(args.output)
    target.parent.mkdir(parents=True, exist_ok=True)
    rows = []
    for path in media_paths(args.input):
        try:
            row = metadata(path, args.ffprobe, args.packet_bytes)
            row["category"] = category(row)
            rows.append(row)
            print(f'{row["category"]:20} {row["duration_seconds"]/60:7.1f} min  {path}')
        except (ValueError, subprocess.CalledProcessError) as error:
            print(f"SKIP {path}: {error}", file=sys.stderr)
    with target.open("w", encoding="utf-8") as handle:
        for row in rows:
            handle.write(json.dumps(row, sort_keys=True) + "\n")
    print(f"Wrote {len(rows)} titles to {target}")


def ffmpeg_run(argv, log_path):
    start = time.monotonic()
    with log_path.open("w", encoding="utf-8") as log:
        result = subprocess.run(argv, stdout=subprocess.DEVNULL, stderr=log)
    elapsed = time.monotonic() - start
    if result.returncode:
        raise RuntimeError(f"FFmpeg exit {result.returncode}; see {log_path}")
    return elapsed


def make_reference(ffmpeg, path, start, duration, output, log):
    pending = output.with_name("reference.partial.mkv")
    ffmpeg_run([ffmpeg, "-hide_banner", "-nostdin", "-y", "-ss", str(start), "-i", str(path),
                "-t", str(duration), "-map", "0:v:0", "-an", "-sn", "-dn", "-fps_mode", "passthrough",
                "-c:v", "ffv1", "-level", "3", str(pending)], log)
    os.replace(pending, output)


def encode_args(ffmpeg, source, start, duration, output, profile, quality, threads):
    codec, profile_args = PROFILES[profile]
    argv = [ffmpeg, "-hide_banner", "-nostdin", "-y", "-ss", str(start), "-i", str(source),
            "-t", str(duration), "-map", "0:v:0", "-an", "-sn", "-dn", "-fps_mode", "passthrough"]
    if profile.startswith("nvenc"):
        argv += ["-pix_fmt", "p010le", "-c:v", codec] + profile_args + [
            "-rc", "vbr", "-cq", str(quality), "-b:v", "0", "-multipass", "fullres",
            "-bf", "4", "-b_ref_mode", "middle", "-split_encode_mode", "disabled"]
    else:
        argv += ["-pix_fmt", "yuv420p10le", "-c:v", codec] + profile_args + ["-crf", str(quality)]
        if threads:
            argv += ["-svtav1-params", f"lp={threads}"]
    return argv + [str(output)]


def vmaf(ffmpeg, distorted, reference, output_json, log):
    filter_arg = f"libvmaf=log_fmt=json:log_path={output_json}:n_subsample=1"
    ffmpeg_run([ffmpeg, "-hide_banner", "-nostdin", "-i", str(distorted), "-i", str(reference),
                "-lavfi", filter_arg, "-f", "null", "-"], log)
    data = json.loads(output_json.read_text())
    scores = [f["metrics"]["vmaf"] for f in data["frames"]]
    scores.sort()
    return {
        "vmaf_mean": data["pooled_metrics"]["vmaf"]["mean"],
        "vmaf_p5": scores[max(0, math.ceil(len(scores) * 0.05) - 1)],
        "vmaf_min": scores[0], "scored_frames": len(scores),
    }


def frames(path, ffprobe):
    result = command([ffprobe, "-v", "error", "-select_streams", "v:0", "-count_frames",
                      "-show_entries", "stream=nb_read_frames", "-of", "default=nw=1:nk=1", str(path)])
    return int(result.stdout.strip())


def run(args):
    output = Path(args.output).resolve()
    output.mkdir(parents=True, exist_ok=True)
    environment = output / "environment.json"
    if not environment.exists():
        versions = {"ffmpeg": command([args.ffmpeg, "-version"]).stdout.splitlines()[0],
                    "ffprobe": command([args.ffprobe, "-version"]).stdout.splitlines()[0],
                    "platform": platform.platform(), "logical_cpus": os.cpu_count()}
        try:
            versions["nvidia_gpu"] = command(["nvidia-smi", "--query-gpu=name,driver_version",
                                              "--format=csv,noheader"]).stdout.strip()
        except (FileNotFoundError, subprocess.CalledProcessError):
            versions["nvidia_gpu"] = "unavailable"
        environment.write_text(json.dumps(versions, indent=2) + "\n", encoding="utf-8")
    result_file = output / "results.jsonl"
    done = {(r["source"], r.get("source_mtime_ns"), r["start"], r["profile"], r["quality"])
            for r in jsonl(result_file)}
    paths = media_paths(args.input)
    if args.max_files:
        paths = paths[:args.max_files]
    profiles = args.profiles.split(",")
    if any(p not in PROFILES for p in profiles):
        raise ValueError(f"Unknown profile in {profiles}; choose from {list(PROFILES)}")
    for path in paths:
        row = metadata(path, args.ffprobe, packets=True)
        if row["hdr"]:
            print(f"SKIP HDR for SDR VMAF model: {path}")
            continue
        if row["codec"] not in {"h264", "hevc"}:
            print(f"PILOT only (source codec {row['codec']}): {path}")
        key = hashlib.sha256(str(path).encode()).hexdigest()[:12]
        for start, length in windows(row["duration_seconds"], args.window, args.windows):
            pending = [(profile, quality) for profile in profiles
                       for quality in (args.nvenc_qualities if profile.startswith("nvenc") else args.svt_qualities)
                       if (str(path), row["mtime_ns"], start, profile, quality) not in done]
            if not pending:
                continue
            work = output / f"{key}-{int(start * 1000)}"
            work.mkdir(exist_ok=True)
            reference = work / "reference.mkv"
            if not reference.exists():
                print(f"Reference {path.name} {start:.1f}s +{length:.1f}s", flush=True)
                make_reference(args.ffmpeg, path, start, length, reference, work / "reference.log")
            reference_frames = frames(reference, args.ffprobe)
            for profile in profiles:
                qualities = args.nvenc_qualities if profile.startswith("nvenc") else args.svt_qualities
                for quality in qualities:
                    identity = (str(path), row["mtime_ns"], start, profile, quality)
                    if identity in done:
                        continue
                    name = f"{profile}-q{quality}"
                    encoded = work / f"{name}.mkv"
                    timings = []
                    print(f"Encode {path.name} @{start:.1f}s {name}", flush=True)
                    for repeat in range(args.repeats):
                        timings.append(ffmpeg_run(encode_args(args.ffmpeg, path, start, length, encoded, profile, quality, args.threads),
                                                  work / f"{name}-repeat{repeat + 1}.log"))
                    encoded_frames = frames(encoded, args.ffprobe)
                    if encoded_frames != reference_frames:
                        raise RuntimeError(f"Frame mismatch {name}: reference={reference_frames}, output={encoded_frames}")
                    score = vmaf(args.ffmpeg, encoded, reference, work / f"{name}-vmaf.json", work / f"{name}-vmaf.log")
                    if score["scored_frames"] != reference_frames:
                        raise RuntimeError(f"VMAF frame mismatch {name}: {score['scored_frames']} vs {reference_frames}")
                    result = {
                        "source": str(path), "category": category(row), "source_codec": row["codec"],
                        "source_file_bytes": row["file_bytes"], "source_mtime_ns": row["mtime_ns"],
                        "source_video_bytes": row["video_packet_bytes"],
                        "source_duration": row["duration_seconds"], "width": row["width"], "height": row["height"],
                        "start": start, "window_seconds": length, "reference_frames": reference_frames,
                        "profile": profile, "quality": quality, "encode_seconds": timings,
                        "video_bytes": video_packet_bytes(encoded, args.ffprobe), **score,
                        "ffmpeg": args.ffmpeg, "ffprobe": args.ffprobe,
                    }
                    append_jsonl(result_file, result)
                    done.add(identity)
                    print(f"  {result['video_bytes']/1e6:.2f} MB / VMAF {score['vmaf_mean']:.2f} / {statistics.median(timings):.1f}s", flush=True)
                    if not args.keep_encodes:
                        encoded.unlink()
            if not args.keep_references:
                reference.unlink()
    print(f"Results: {result_file}")


def interpolate(points, target):
    """Estimate a point at target VMAF; never extrapolate beyond tested qualities."""
    ordered = sorted(points, key=lambda p: p["vmaf_mean"])
    for point in ordered:
        if math.isclose(point["vmaf_mean"], target, abs_tol=1e-6):
            return {"video_bytes": point["video_bytes"],
                    "encode_seconds": statistics.median(point["encode_seconds"]),
                    "vmaf_p5": point["vmaf_p5"]}
    if len(ordered) < 2 or target < ordered[0]["vmaf_mean"] or target > ordered[-1]["vmaf_mean"]:
        return None
    for low, high in zip(ordered, ordered[1:]):
        if low["vmaf_mean"] <= target <= high["vmaf_mean"]:
            span = high["vmaf_mean"] - low["vmaf_mean"]
            fraction = 0 if span == 0 else (target - low["vmaf_mean"]) / span
            # Log-space interpolation approximates a rate-distortion curve.
            size = math.exp(math.log(low["video_bytes"]) * (1 - fraction) + math.log(high["video_bytes"]) * fraction)
            seconds = statistics.median(low["encode_seconds"]) * (1 - fraction) + statistics.median(high["encode_seconds"]) * fraction
            return {"video_bytes": size, "encode_seconds": seconds,
                    "vmaf_p5": low["vmaf_p5"] * (1 - fraction) + high["vmaf_p5"] * fraction}
    return None


def report(args):
    rows = jsonl(Path(args.results))
    if not rows:
        raise ValueError("No benchmark rows")
    groups = {}
    for row in rows:
        groups.setdefault((row["source"], row["start"]), {}).setdefault(row["profile"], []).append(row)
    matches = []
    for (source, start), profiles in groups.items():
        example = next(iter(profiles.values()))[0]
        candidates = {name: interpolate(points, args.target_vmaf) for name, points in profiles.items()}
        for name, estimate in candidates.items():
            if estimate is None:
                continue
            matches.append({"source": source, "start": start, "category": example["category"],
                            "profile": name, "target_vmaf": args.target_vmaf,
                            "window_seconds": example["window_seconds"],
                            "source_duration": example["source_duration"],
                            "source_video_bytes": example["source_video_bytes"], **estimate})
    output = Path(args.output)
    output.parent.mkdir(parents=True, exist_ok=True)
    with output.open("w", newline="", encoding="utf-8") as handle:
        if matches:
            writer = csv.DictWriter(handle, fieldnames=matches[0].keys())
            writer.writeheader()
            writer.writerows(matches)
    print(f"Matched quality rows: {len(matches)} -> {output}")
    by_window = {}
    for row in matches:
        by_window.setdefault((row["source"], row["start"]), []).append(row)
    comparisons = []
    for alternatives in by_window.values():
        gpu = [r for r in alternatives if r["profile"].startswith("nvenc")]
        cpu = [r for r in alternatives if r["profile"].startswith("svt")]
        if not gpu or not cpu:
            continue
        best_cpu = min(cpu, key=lambda r: r["video_bytes"])
        eligible = [r for r in gpu if r["video_bytes"] <= best_cpu["video_bytes"] * (1 + args.max_gpu_premium / 100)]
        if not eligible:
            continue
        best_gpu = min(eligible, key=lambda r: r["encode_seconds"])
        comparisons.append((best_gpu, best_cpu))
    if comparisons:
        premiums = [(g["video_bytes"] / c["video_bytes"] - 1) * 100 for g, c in comparisons]
        speedups = [c["encode_seconds"] / g["encode_seconds"] for g, c in comparisons]
        print(f"Balanced GPU candidates: {len(comparisons)}/{len(by_window)} windows; "
              f"median size premium {statistics.median(premiums):+.1f}%; "
              f"median encode speedup {statistics.median(speedups):.1f}x")
    else:
        print("No matched GPU/CPU windows within the storage premium; expand the quality sweep or revise the threshold.")
    if args.inventory:
        library = jsonl(Path(args.inventory))
        # Report projections by source class. No estimate for classes without a measured window.
        for label, chosen in (("GPU", [g for g, _ in comparisons]), ("CPU", [c for _, c in comparisons])):
            by_category = {}
            for row in chosen:
                if row["source_video_bytes"]:
                    ratio = (row["video_bytes"] / row["window_seconds"]) / (row["source_video_bytes"] / row["source_duration"])
                    realtime = row["encode_seconds"] / row["window_seconds"]
                    by_category.setdefault(row["category"], {}).setdefault(row["source"], []).append((ratio, realtime))
            # Equal weight per sampled title; repeated windows must not make one title dominate.
            class_samples = {
                name: [(statistics.median(x[0] for x in windows), statistics.median(x[1] for x in windows))
                       for windows in titles.values()]
                for name, titles in by_category.items() if len(titles) >= args.min_sample_titles
            }
            covered = [r for r in library if r["category"] in class_samples and r["video_packet_bytes"] is not None]
            uncovered = [r for r in library if r not in covered]
            if not covered:
                print(f"{label} projection: unavailable; {len(uncovered)} titles lack a class with "
                      f"{args.min_sample_titles} distinct matched-quality sample titles and exact packet sizes")
                continue
            before = sum(r["file_bytes"] for r in covered)
            def project_size(reducer):
                return sum(r["file_bytes"] - r["video_packet_bytes"] + r["video_packet_bytes"] *
                           reducer(sample[0] for sample in class_samples[r["category"]]) for r in covered)
            def project_hours(reducer):
                return sum(r["duration_seconds"] *
                           reducer(sample[1] for sample in class_samples[r["category"]]) / 3600 for r in covered)
            after, low, high = project_size(statistics.median), project_size(min), project_size(max)
            hours, low_hours, high_hours = project_hours(statistics.median), project_hours(min), project_hours(max)
            print(f"{label} projection: {len(covered)} covered titles, {len(uncovered)} uncovered; "
                  f"covered original {before/1e9:.2f} GB -> {after/1e9:.2f} GB "
                  f"(sample-title range {low/1e9:.2f}–{high/1e9:.2f}), "
                  f"{hours:.2f} serial encode hours (range {low_hours:.2f}–{high_hours:.2f})")
        print("Projection excludes mux overhead changes, stream conversions, concurrency effects, and uncovered classes.")


def throughput(args):
    if args.profile not in PROFILES or not args.profile.startswith("nvenc"):
        raise ValueError("Throughput mode requires nvenc-p5 or nvenc-p7")
    paths = media_paths(args.input)
    if not paths:
        raise ValueError("No video input")
    paths = (paths * 2)[:2]
    output = Path(args.output).resolve()
    output.mkdir(parents=True, exist_ok=True)
    tasks = []
    for index, path in enumerate(paths):
        remaining = metadata(path, args.ffprobe)["duration_seconds"] - args.start
        if remaining <= 0:
            raise ValueError(f"Start {args.start}s is beyond {path}")
        duration = min(args.window, remaining)
        tasks.append((index, path, duration))
    results = []
    for repeat in range(args.repeats):
        by_mode = {}
        for mode, workers in (("serial", 1), ("parallel", 2)):
            def one(task):
                index, path, duration = task
                encoded = output / f"{mode}-{repeat}-{index}.mkv"
                log = output / f"{mode}-{repeat}-{index}.log"
                seconds = ffmpeg_run(encode_args(args.ffmpeg, path, args.start, duration, encoded,
                                                  args.profile, args.quality, args.threads), log)
                return {"source": str(path), "encoded": str(encoded), "encode_seconds": seconds,
                        "window_seconds": duration}
            began = time.monotonic()
            with ThreadPoolExecutor(max_workers=workers) as pool:
                jobs = list(pool.map(one, tasks))
            wall_seconds = time.monotonic() - began
            for result in jobs:
                encoded = Path(result.pop("encoded"))
                result["frames"] = frames(encoded, args.ffprobe)
                result["video_bytes"] = video_packet_bytes(encoded, args.ffprobe)
                if not args.keep_encodes:
                    encoded.unlink()
            by_mode[mode] = {"wall_seconds": wall_seconds, "jobs": jobs}
        for serial, parallel in zip(by_mode["serial"]["jobs"], by_mode["parallel"]["jobs"]):
            if serial["frames"] != parallel["frames"]:
                raise RuntimeError("Serial/parallel output frame counts differ")
        result = {"repeat": repeat + 1, "profile": args.profile, "quality": args.quality,
                  "serial": by_mode["serial"], "parallel": by_mode["parallel"]}
        append_jsonl(output / "throughput.jsonl", result)
        results.append(result)
        print(f"Repeat {repeat + 1}: serial {by_mode['serial']['wall_seconds']:.1f}s, "
              f"two sessions {by_mode['parallel']['wall_seconds']:.1f}s")
    speedups = [r["serial"]["wall_seconds"] / r["parallel"]["wall_seconds"] for r in results]
    print(f"Median two-session throughput speedup: {statistics.median(speedups):.2f}x; see {output / 'throughput.jsonl'}")


def main():
    parser = argparse.ArgumentParser(description=__doc__)
    sub = parser.add_subparsers(dest="action", required=True)
    inv = sub.add_parser("inventory", help="Read-only metadata and optional exact video packet scan")
    inv.add_argument("--input", action="append", required=True, help="File or folder, repeatable")
    inv.add_argument("--output", default="build/av1-lab/inventory.jsonl")
    inv.add_argument("--ffprobe", default="ffprobe")
    inv.add_argument("--packet-bytes", action="store_true", help="Scan packets for exact video payload size (slower)")
    inv.set_defaults(func=inventory)
    bench = sub.add_parser("run", help="Create lossless windows and benchmark video-only encodes")
    bench.add_argument("--input", action="append", required=True, help="File or folder, repeatable")
    bench.add_argument("--output", default="build/av1-lab")
    bench.add_argument("--ffmpeg", default="ffmpeg")
    bench.add_argument("--ffprobe", default="ffprobe")
    bench.add_argument("--window", type=float, default=60, help="Window seconds (default 60)")
    bench.add_argument("--windows", type=int, default=3, help="Windows per title (default 3)")
    bench.add_argument("--repeats", type=int, default=3)
    bench.add_argument("--profiles", default=",".join(PROFILES))
    bench.add_argument("--nvenc-qualities", type=lambda s: [float(x) for x in s.split(",")], default=DEFAULT_QUALITIES["nvenc"])
    bench.add_argument("--svt-qualities", type=lambda s: [int(x) for x in s.split(",")], default=DEFAULT_QUALITIES["svt"])
    bench.add_argument("--threads", type=int, default=0, help="SVT logical processors; 0 uses SVT default")
    bench.add_argument("--max-files", type=int, default=0)
    bench.add_argument("--keep-encodes", action="store_true")
    bench.add_argument("--keep-references", action="store_true")
    bench.set_defaults(func=run)
    rep = sub.add_parser("report", help="Matched VMAF comparison and optional library estimate")
    rep.add_argument("--results", default="build/av1-lab/results.jsonl")
    rep.add_argument("--inventory", help="Inventory with --packet-bytes for library projection")
    rep.add_argument("--output", default="build/av1-lab/matched.csv")
    rep.add_argument("--target-vmaf", type=float, default=95)
    rep.add_argument("--max-gpu-premium", type=float, default=15, help="Largest NVENC byte premium vs best SVT, percent")
    rep.add_argument("--min-sample-titles", type=int, default=3, help="Distinct measured titles required per source class")
    rep.set_defaults(func=report)
    parallel = sub.add_parser("throughput", help="Compare two NVENC jobs serially and concurrently")
    parallel.add_argument("--input", action="append", required=True, help="One or two files, repeatable")
    parallel.add_argument("--output", default="build/av1-lab/throughput")
    parallel.add_argument("--ffmpeg", default="ffmpeg")
    parallel.add_argument("--ffprobe", default="ffprobe")
    parallel.add_argument("--profile", default="nvenc-p7")
    parallel.add_argument("--quality", type=float, default=22)
    parallel.add_argument("--start", type=float, default=0)
    parallel.add_argument("--window", type=float, default=300)
    parallel.add_argument("--repeats", type=int, default=3)
    parallel.add_argument("--threads", type=int, default=0)
    parallel.add_argument("--keep-encodes", action="store_true")
    parallel.set_defaults(func=throughput)
    args = parser.parse_args()
    if args.action == "run" and (args.window <= 0 or args.windows <= 0 or args.repeats <= 0):
        parser.error("--window, --windows and --repeats must be positive")
    if args.action == "throughput" and (args.window <= 0 or args.repeats <= 0):
        parser.error("--window and --repeats must be positive")
    args.func(args)


if __name__ == "__main__":
    main()

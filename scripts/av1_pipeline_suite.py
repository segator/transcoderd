#!/usr/bin/env python3
"""Offline AV1 bit-depth, decode, scaling and contention experiment.

Manifest: [{"id": "scene", "source": "/local/compressed-excerpt.mkv"}].
Never modifies originals or worker configuration. Retains argv, probes, frame
metrics, repeated timings and resource samples in a private output directory.
"""

import argparse
import csv
from fractions import Fraction
import hashlib
import json
import math
import os
from pathlib import Path
import re
import signal
import statistics
import subprocess
import threading
import time

import av1_lab as lab


SCALE = "scale='min(1920,iw)':-1:force_original_aspect_ratio=decrease"
GPU = ["-c:v", "av1_nvenc", "-preset", "p7", "-tune", "uhq", "-rc", "vbr",
       "-b:v", "0", "-multipass", "fullres", "-bf", "4", "-b_ref_mode", "middle",
       "-split_encode_mode", "disabled", "-qmin", "0", "-qmax", "100"]
MODES = ["cpu-scale", "cpu-native", "upload-cuda-scale", "nvdec-cpu-scale",
         "nvdec-cuda-scale", "nvdec-native", "nvdec-direct"]


def save(path, data):
    path = Path(path)
    tmp = path.with_suffix(path.suffix + ".partial")
    tmp.write_text(json.dumps(data, indent=2, allow_nan=False) + "\n")
    tmp.replace(path)


def sha(path):
    with Path(path).open("rb") as handle:
        return hashlib.file_digest(handle, "sha256").hexdigest()


def run(command, log):
    with Path(log).open("w") as handle:
        subprocess.run(command, stdout=handle, stderr=subprocess.STDOUT, check=True)


def depth(stream):
    raw = stream.get("bits_per_raw_sample")
    if raw and int(raw):
        return int(raw)
    fmt = stream["pix_fmt"]
    return 10 if "10" in fmt or fmt.startswith("p010") else 12 if "12" in fmt else 8


def proc(pid):
    try:
        parts = Path(f"/proc/{pid}/stat").read_text().rsplit(")", 1)[1].split()
        return {"cpu_seconds": (int(parts[11]) + int(parts[12])) / os.sysconf("SC_CLK_TCK"),
                "rss_bytes": int(parts[21]) * os.sysconf("SC_PAGE_SIZE")}
    except (OSError, ValueError, IndexError):
        return None


def gpu():
    command = ["nvidia-smi", "--query-gpu=utilization.gpu,utilization.encoder,utilization.decoder,"
               "memory.used,temperature.gpu,power.draw", "--format=csv,noheader,nounits"]
    result = subprocess.run(command, capture_output=True, text=True, timeout=10)
    return result.stdout.strip() if result.returncode == 0 else None


def progress(path):
    try:
        values = [int(line.split("=", 1)[1]) for line in Path(path).read_text().splitlines()
                  if line.startswith("frame=")]
        return values[-1] if values else 0
    except OSError:
        return 0


def command(args, scene, encoder, mode, bits, quality, frames, output, progress_path=None, loop=False):
    source = scene["source"]
    stream = scene["stream"]
    pixel = ("p010le" if bits == 10 else "nv12") if encoder == "gpu" else (
        "yuv420p10le" if bits == 10 else "yuv420p")
    cmd = [args.ffmpeg, "-hide_banner", "-nostdin", "-nostats", "-v", "info", "-n"]
    if progress_path:
        cmd += ["-progress", str(progress_path)]
    if loop:
        cmd += ["-stream_loop", "-1"]
    if mode.startswith("nvdec"):
        cmd += ["-hwaccel", "cuda", "-hwaccel_output_format", "cuda"]
    cmd += ["-threads", "0", "-i", source, "-map", "0:v:0", "-an", "-sn", "-dn",
            "-map_metadata", "-1", "-map_chapters", "-1", "-fps_mode", "passthrough"]
    filters = []
    if mode == "cpu-scale":
        filters.append(SCALE)
    elif mode == "upload-cuda-scale":
        cmd[1:1] = ["-init_hw_device", "cuda=bench:0", "-filter_hw_device", "bench"]
        filters += [f"format={pixel}", "hwupload_cuda",
                    f"scale_cuda=w='min(1920,iw)':h=-2:format={pixel}:interp_algo=bicubic"]
    elif mode == "nvdec-cpu-scale":
        native = "p010le" if depth(stream) == 10 else "nv12"
        filters += ["hwdownload", f"format={native}", SCALE]
    elif mode == "nvdec-cuda-scale":
        filters.append(f"scale_cuda=w='min(1920,iw)':h=-2:format={pixel}:interp_algo=bicubic")
    elif mode == "nvdec-native":
        # Conversion only: dimensions stay unchanged; matching surfaces bypass.
        filters.append(f"scale_cuda=format={pixel}:passthrough=1")
    elif mode == "nvdec-direct" and depth(stream) == 10 and bits == 8:
        raise ValueError("Direct 10-to-8 requires an explicit conversion; use nvdec-native")
    if stream.get("color_space") == "gbr" and stream["pix_fmt"].startswith(("yuv", "nv", "p0")):
        filters.append("setparams=colorspace=unknown")
    if filters:
        cmd += ["-vf", ",".join(filters)]
    hardware_output = mode in {"upload-cuda-scale", "nvdec-cuda-scale", "nvdec-native", "nvdec-direct"}
    if not hardware_output:
        cmd += ["-pix_fmt", pixel]
    if encoder == "gpu":
        cmd += GPU + ["-cq", str(quality)]
        if mode == "nvdec-direct" and depth(stream) == 8 and bits == 10:
            cmd += ["-highbitdepth", "1"]
    else:
        cmd += ["-c:v", "libsvtav1", "-preset", "4", "-crf", str(quality)]
    if frames:
        cmd += ["-frames:v", str(frames)]
    if output == "-":
        cmd += ["-f", "null", "-"]
    else:
        cmd.append(str(output))
    return cmd


def measure(args, name, jobs, background=()):
    target = args.output / name
    record_path = target.with_suffix(".json")
    commands = []
    for index, job in enumerate(jobs):
        cmd = list(job["command"])
        if "-progress" not in cmd:
            cmd[1:1] = ["-progress", str(target.with_suffix(f".job{index}.progress"))]
        commands.append(cmd)
    if record_path.exists():
        result = json.loads(record_path.read_text())
        if result["commands"] != commands:
            raise ValueError(f"Cached command differs: {name}")
        for job in jobs:
            if job.get("output") and not Path(job["output"]).is_file():
                raise ValueError(f"Missing cached output: {name}")
        return result
    if target.with_suffix(".started.json").exists():
        raise ValueError(f"Incomplete run; inspect and move its artifacts before retrying: {name}")
    save(target.with_suffix(".started.json"), {"commands": commands})
    print("RUN", name, flush=True)
    handles, processes, samples, finished = [], [], [], {}
    start = time.monotonic()
    def wait_process(index, process):
        code = process.wait()
        finished[index] = {"seconds": time.monotonic() - start, "exit": code}

    threads = []
    try:
        for index, job in enumerate(jobs):
            handle = target.with_suffix(f".job{index}.log").open("w")
            handles.append(handle)
            process = subprocess.Popen(commands[index], stdout=handle, stderr=subprocess.STDOUT,
                                       start_new_session=True)
            processes.append(process)
            thread = threading.Thread(target=wait_process, args=(index, process))
            threads.append(thread)
            thread.start()
        while len(finished) < len(processes):
            now = time.monotonic()
            samples.append({"seconds": now - start, "gpu": gpu(),
                            "processes": [proc(p.pid) for p in processes],
                            "background": [proc(p.pid) for p in background]})
            for index, result in list(finished.items()):
                if result["exit"]:
                    raise RuntimeError(f"Encode failed: {name} job{index}; inspect its log")
            if len(finished) < len(processes):
                time.sleep(.4)
    finally:
        for process in processes:
            if process.poll() is None:
                os.killpg(process.pid, signal.SIGTERM)
                try:
                    process.wait(timeout=15)
                except subprocess.TimeoutExpired:
                    os.killpg(process.pid, signal.SIGKILL)
                    process.wait()
        for handle in handles:
            handle.close()
        for thread in threads:
            thread.join()
    for index, cmd in enumerate(commands):
        if finished[index]["exit"]:
            raise RuntimeError(f"Encode failed: {name} job{index}; inspect its log")
        if "-frames:v" in cmd:
            expected = int(cmd[cmd.index("-frames:v") + 1])
            actual = progress(cmd[cmd.index("-progress") + 1])
            if actual != expected:
                raise RuntimeError(f"Encode frame count {actual} != {expected}: {name} job{index}")
    result = {"commands": commands, "wall_seconds": max(r["seconds"] for r in finished.values()),
              "jobs": finished, "samples": samples}
    save(record_path, result)
    return result


def validate(args, output, expected, bits, dimensions):
    info = lab.probe(output, args.ffprobe)
    stream = next(s for s in info["streams"] if s["codec_type"] == "video")
    if (stream["codec_name"], depth(stream), stream["width"], stream["height"]) != (
            "av1", bits, *dimensions):
        raise ValueError(f"Unexpected output format: {output}: {stream}")
    result = subprocess.run([args.ffprobe, "-v", "error", "-select_streams", "v:0", "-show_frames",
                             "-show_entries", "frame=best_effort_timestamp_time", "-of", "json", str(output)],
                            capture_output=True, text=True, check=True)
    frames = json.loads(result.stdout)["frames"]
    times = [float(f["best_effort_timestamp_time"]) for f in frames]
    if len(times) != expected or any(b <= a for a, b in zip(times, times[1:])):
        raise ValueError(f"Frame count or timing mismatch: {output}")
    run([args.ffmpeg, "-v", "error", "-xerror", "-i", str(output), "-map", "0:v:0",
         "-f", "null", "-"], output.with_suffix(".decode.log"))
    return {"probe": info, "frame_count": len(times), "timeline": times,
            "mkv_bytes": output.stat().st_size,
            "video_bytes": lab.video_packet_bytes(output, args.ffprobe)}


def score(args, scene, encoded):
    target = encoded.with_suffix(".metrics.json")
    if target.exists():
        return json.loads(target.read_text())
    if scene["hdr"]:
        result = {"scored": False, "reason": "HDR requires a separate color-managed evaluation"}
        save(target, result)
        return result
    model = "vmaf_4k_v0.6.1" if scene["stream"]["width"] > 1920 else "vmaf_v0.6.1"
    vf = encoded.with_suffix(".vmaf.json")
    ssim = encoded.with_suffix(".ssim.txt")
    # Explicit 10-bit scoring keeps 10-to-8 quantization visible to the metrics.
    graph = ("[0:v]format=yuv420p10le,settb=AVTB,setpts=PTS-STARTPTS,split=2[d0][d1];"
             f"[1:v]trim=end_frame={args.quality_frames},format=yuv420p10le,"
             "settb=AVTB,setpts=PTS-STARTPTS,split=2[r0][r1];"
             f"[d0][r0]libvmaf=model=version={model}:n_threads=6:n_subsample=1:"
             f"log_fmt=json:log_path={vf.name}[v];"
             f"[d1][r1]ssim=stats_file={ssim.name}[s]")
    cmd = [args.ffmpeg, "-v", "info", "-nostdin", "-i", str(encoded), "-i", scene["source"],
           "-filter_complex", graph, "-map", "[v]", "-map", "[s]", "-f", "null", "-"]
    with encoded.with_suffix(".metric.log").open("w") as log:
        subprocess.run(cmd, cwd=args.output, stdout=log, stderr=subprocess.STDOUT, check=True)
    data = json.loads(vf.read_text())
    frames = data["frames"]
    values = [f["metrics"]["vmaf"] for f in frames]
    if len(values) != args.quality_frames or [f["frameNum"] for f in frames] != list(range(len(frames))):
        raise ValueError(f"VMAF frame count/indices mismatch: {encoded}")
    if any(not math.isfinite(v) for v in values):
        raise ValueError("Non-finite VMAF")
    ordered = sorted(values)
    ssim_values = [float(re.search(r"\bAll:([^\s]+)", line)[1])
                   for line in ssim.read_text().splitlines()]
    if len(ssim_values) != len(values) or any(not math.isfinite(v) for v in ssim_values):
        raise ValueError("SSIM frame count/values mismatch")
    result = {"scored": True, "model": model, "metric_pixel_format": "yuv420p10le",
              "frames": len(values), "vmaf_mean": statistics.mean(values),
              "vmaf_p5": ordered[math.ceil(.05 * len(values)) - 1],
              "vmaf_p1": ordered[math.ceil(.01 * len(values)) - 1],
              "vmaf_min": min(values), "worst_frame": values.index(min(values)),
              "ssim_mean": statistics.mean(ssim_values), "command": cmd}
    save(target, result)
    return result


def quality(args, scenes):
    rows = []
    for scene in scenes:
        for encoder, qualities in (("cpu", [24, 26, 28]), ("gpu", [36, 38, 40])):
            for q in qualities:
                for bits in (10, 8):
                    name = f"quality-{scene['id']}-{encoder}-{bits}bit-q{q}"
                    output = args.output / (name + ".mkv")
                    cmd = command(args, scene, encoder, "cpu-native", bits, q, args.quality_frames, output)
                    timing = measure(args, name, [{"command": cmd, "output": str(output)}])
                    stream = scene["stream"]
                    checked = validate(args, output, args.quality_frames, bits, (stream["width"], stream["height"]))
                    metrics = score(args, scene, output)
                    rows.append({"scene": scene["id"], "source_depth": depth(stream), "hdr": scene["hdr"],
                                 "encoder": encoder, "bits": bits, "quality": q,
                                 "seconds": timing["wall_seconds"], "video_bytes": checked["video_bytes"],
                                 "mkv_bytes": checked["mkv_bytes"], "output": str(output), **metrics})
                    save(args.output / "quality-results.json", rows)
                    print("QUALITY", name, checked["video_bytes"], metrics.get("vmaf_mean"), flush=True)


def decode_check(args, scenes):
    for scene in scenes:
        results = {}
        pixel = "yuv420p10le" if depth(scene["stream"]) == 10 else "yuv420p"
        for mode in ("cpu", "nvdec"):
            target = args.output / f"decode-{scene['id']}-{mode}.framemd5"
            cmd = [args.ffmpeg, "-v", "error", "-nostdin"]
            if mode == "nvdec":
                # Return decoded surfaces to system RAM without resizing.
                cmd += ["-hwaccel", "cuda"]
            cmd += ["-i", scene["source"], "-map", "0:v:0", "-an", "-sn", "-frames:v",
                    str(args.quality_frames), "-pix_fmt", pixel, "-fps_mode", "passthrough",
                    "-f", "framemd5", "-n", str(target)]
            if not target.exists():
                run(cmd, target.with_suffix(".log"))
            values = [line.rsplit(",", 1)[1].strip() for line in target.read_text().splitlines()
                      if line and not line.startswith("#")]
            if len(values) != args.quality_frames:
                raise ValueError("Decoded source frame count mismatch")
            results[mode] = {"sha256": hashlib.sha256("\n".join(values).encode()).hexdigest(), "command": cmd}
        results["identical_pixels"] = results["cpu"]["sha256"] == results["nvdec"]["sha256"]
        save(args.output / f"decode-{scene['id']}-comparison.json", results)
        print("DECODE", scene["id"], results["identical_pixels"], flush=True)


def pipeline_quality(args, scenes):
    rows = []
    for scene in scenes:
        for bits in (10, 8):
            for mode in ("nvdec-native", "nvdec-direct"):
                if mode == "nvdec-direct" and depth(scene["stream"]) == 10 and bits == 8:
                    continue
                name = f"pipeline-{scene['id']}-{mode}-{bits}bit"
                output = args.output / (name + ".mkv")
                cmd = command(args, scene, "gpu", mode, bits, 38, args.quality_frames, output)
                measure(args, name, [{"command": cmd, "output": str(output)}])
                stream = scene["stream"]
                checked = validate(args, output, args.quality_frames, bits, (stream["width"], stream["height"]))
                metrics = score(args, scene, output)
                rows.append({"scene": scene["id"], "mode": mode, "bits": bits,
                             "video_bytes": checked["video_bytes"], "mkv_bytes": checked["mkv_bytes"],
                             "output": str(output), **metrics})
                save(args.output / "pipeline-quality-results.json", rows)
                print("PIPELINE QUALITY", name, checked["video_bytes"], metrics.get("vmaf_mean"), flush=True)


def audit(args, scenes):
    rows = []
    for scene in scenes:
        result = subprocess.run([args.ffprobe, "-v", "error", "-select_streams", "v:0",
                                 "-read_intervals", f"%+#{args.quality_frames + 128}",
                                 "-show_frames", "-show_entries", "frame=best_effort_timestamp_time",
                                 "-of", "json", scene["source"]], capture_output=True, text=True, check=True)
        frames = json.loads(result.stdout)["frames"][:args.quality_frames]
        source_times = [float(f["best_effort_timestamp_time"]) for f in frames]
        if len(source_times) != args.quality_frames or any(b <= a for a, b in zip(source_times, source_times[1:])):
            raise ValueError(f"Invalid source timeline: {scene['id']}")
        outputs = list(args.output.glob(f"quality-{scene['id']}-*.mkv")) + list(args.output.glob(f"pipeline-{scene['id']}-*.mkv"))
        for output in outputs:
            bits = 10 if "-10bit" in output.stem else 8
            checked = validate(args, output, args.quality_frames, bits,
                               (scene["stream"]["width"], scene["stream"]["height"]))
            output_times = checked["timeline"]
            delta = max(abs((a - source_times[0]) - (b - output_times[0])) for a, b in zip(source_times, output_times))
            if delta > .0011:
                raise ValueError(f"Changed frame timeline ({delta}s): {output}")
            stream = next(s for s in checked["probe"]["streams"] if s["codec_type"] == "video")
            for key in ("color_transfer", "color_primaries", "color_space"):
                wanted = scene["stream"].get(key)
                if wanted and wanted not in {"unknown", "unspecified", "gbr"} and stream.get(key) != wanted:
                    raise ValueError(f"Changed {key}: {output}")
            rows.append({"scene": scene["id"], "output": str(output), "frames": checked["frame_count"],
                         "bits": bits, "max_normalized_timestamp_delta_seconds": delta,
                         "color_metadata_preserved": True})
    save(args.output / "audit.json", rows)
    print("AUDIT", len(rows), "outputs passed", flush=True)


def speed(args, scenes):
    rows = []
    for scene in scenes:
        for encoder in ("cpu", "gpu"):
            selected = args.cpu_modes if encoder == "cpu" else args.gpu_modes
            modes = selected.split(",") if selected else (["cpu-scale", "cpu-native"] if encoder == "cpu" else MODES)
            for repeat in range(args.repeats):
                for bits in (args.bits if repeat % 2 == 0 else list(reversed(args.bits))):
                    for mode in (modes if repeat % 2 == 0 else list(reversed(modes))):
                        if mode == "nvdec-direct" and depth(scene["stream"]) == 10 and bits == 8:
                            continue
                        name = f"speed-{scene['id']}-{encoder}-{mode}-{bits}bit-r{repeat+1}"
                        cmd = command(args, scene, encoder, mode, bits, 26 if encoder == "cpu" else 38,
                                      args.speed_frames, "-")
                        timing = measure(args, name, [{"command": cmd}])
                        rows.append({"scene": scene["id"], "encoder": encoder, "mode": mode,
                                     "bits": bits, "repeat": repeat + 1, "frames": args.speed_frames,
                                     "seconds": timing["wall_seconds"]})
                        save(args.output / "speed-results.json", rows)


def contention(args, scenes):
    rows = []
    scene = scenes[0]
    for cpu_jobs, gpu_jobs in ((1, 1), (1, 2), (2, 2)):
        for repeat in range(args.repeats):
            variants = [(mode, bits) for bits in args.bits for mode in ("cpu-scale", "cpu-native", "nvdec-native", "nvdec-direct")
                        if not (mode == "nvdec-direct" and depth(scene["stream"]) == 10 and bits == 8)]
            if repeat % 2:
                variants.reverse()
            for mode, bits in variants:
                name = f"mixed-{cpu_jobs}cpu-{gpu_jobs}gpu-{mode}-{bits}bit-r{repeat+1}"
                cached = args.output / (name + ".json")
                if cached.exists():
                    rows.append(json.loads(cached.read_text())["summary"])
                    continue
                handles, processes, paths = [], [], []
                try:
                    for index in range(cpu_jobs):
                        path = args.output / f"{name}-load{index}.progress"
                        paths.append(path)
                        cmd = command(args, scene, "cpu", "cpu-native", 10, 26, None, "-", path, loop=True)
                        handle = (args.output / f"{name}-load{index}.log").open("w")
                        handles.append(handle)
                        processes.append(subprocess.Popen(cmd, stdout=handle, stderr=subprocess.STDOUT, start_new_session=True))
                    time.sleep(4)
                    if any(p.poll() is not None for p in processes):
                        raise RuntimeError("CPU load failed before GPU timing")
                    before = [progress(path) for path in paths]
                    cmd = command(args, scene, "gpu", mode, bits, 38, args.speed_frames, "-")
                    timing = measure(args, name, [{"command": cmd} for _ in range(gpu_jobs)], processes)
                    if any(p.poll() is not None for p in processes):
                        raise RuntimeError("CPU load failed during GPU timing")
                    after = [progress(path) for path in paths]
                    duration = timing["wall_seconds"]
                    summary = {"scene": scene["id"], "cpu_jobs": cpu_jobs, "gpu_jobs": gpu_jobs,
                               "mode": mode, "bits": bits, "repeat": repeat + 1, "seconds": duration,
                               "gpu_fps": args.speed_frames * gpu_jobs / duration,
                               "cpu_fps": sum(b - a for a, b in zip(before, after)) / duration,
                               "cpu_frames_before": before, "cpu_frames_after": after}
                    timing["summary"] = summary
                    save(cached, timing)
                    rows.append(summary)
                    save(args.output / "mixed-results.json", rows)
                finally:
                    for process in processes:
                        if process.poll() is None:
                            os.killpg(process.pid, signal.SIGTERM)
                        try:
                            process.wait(timeout=15)
                        except subprocess.TimeoutExpired:
                            os.killpg(process.pid, signal.SIGKILL)
                            process.wait()
                    for handle in handles:
                        handle.close()


def report(args):
    lines = ["# AV1 pipeline experiment", "", "Offline excerpts; no production settings changed.",
             "Timed runs exclude quality scoring. Quality-run durations are screening observations, not repeated speed results.",
             "HDR receives no SDR-VMAF recommendation. Different resolutions are not a matched-quality comparison.", ""]
    qpath = args.output / "quality-results.json"
    if qpath.exists():
        rows = json.loads(qpath.read_text())
        lines += ["## Bit depth at fixed profiles", "", "| Scene | Source | Encoder | Quality | 8-bit bytes change vs 10 | Mean VMAF change | p5 change |", "|---|---|---|---:|---:|---:|---:|"]
        pairs = {}
        for row in rows:
            pairs.setdefault((row["scene"], row["encoder"], row["quality"]), {})[row["bits"]] = row
        for (scene, encoder, q), pair in pairs.items():
            if set(pair) != {8, 10}:
                continue
            a, b = pair[10], pair[8]
            delta = lambda key: f"{b[key]-a[key]:+.3f}" if a.get("scored") and b.get("scored") else "HDR: unscored"
            lines.append(f"| {scene} | {a['source_depth']} bit | {encoder} | {q} | {100*(b['video_bytes']/a['video_bytes']-1):+.2f}% | {delta('vmaf_mean')} | {delta('vmaf_p5')} |")
        lines += ["", "## Estimated 8-bit size at matched mean VMAF", "",
                  "In-range screening interpolation, not additional measured encodes. Confirm near the estimate before selecting a profile.", "",
                  "| Scene | Encoder | 10-bit baseline | Target mean VMAF | Estimated 8-bit bytes change | Estimated p5 change |",
                  "|---|---|---:|---:|---:|---:|"]
        for baseline in rows:
            if baseline["bits"] != 10 or baseline["quality"] != (26 if baseline["encoder"] == "cpu" else 38) or not baseline.get("scored"):
                continue
            points = [{**r, "encode_seconds": [r["seconds"]]} for r in rows
                      if r["scene"] == baseline["scene"] and r["encoder"] == baseline["encoder"]
                      and r["bits"] == 8 and r.get("scored")]
            match = lab.interpolate(points, baseline["vmaf_mean"])
            if match:
                size = 100 * (match["video_bytes"] / baseline["video_bytes"] - 1)
                p5 = match["vmaf_p5"] - baseline["vmaf_p5"]
                lines.append(f"| {baseline['scene']} | {baseline['encoder']} | {baseline['quality']} | {baseline['vmaf_mean']:.3f} | {size:+.2f}% | {p5:+.3f} |")
            else:
                lines.append(f"| {baseline['scene']} | {baseline['encoder']} | {baseline['quality']} | {baseline['vmaf_mean']:.3f} | Outside measured range | — |")
    spath = args.output / "speed-results.json"
    if spath.exists():
        lines += ["", "## Repeated isolated timings", "", "| Scene | Encoder | Mode | Bits | Median seconds | Runs |", "|---|---|---|---:|---:|---:|"]
        groups = {}
        for row in json.loads(spath.read_text()):
            groups.setdefault((row["scene"], row["encoder"], row["mode"], row["bits"]), []).append(row["seconds"])
        for key, values in groups.items():
            lines.append("| " + " | ".join(map(str, key)) + f" | {statistics.median(values):.3f} | {len(values)} |")
    ppath = args.output / "pipeline-quality-results.json"
    if ppath.exists():
        lines += ["", "## Native GPU pipeline quality", "", "| Scene | Mode | Bits | Video bytes | Mean VMAF | p5 VMAF |", "|---|---|---:|---:|---:|---:|"]
        for row in json.loads(ppath.read_text()):
            mean = f"{row['vmaf_mean']:.3f}" if row.get("scored") else "HDR: unscored"
            p5 = f"{row['vmaf_p5']:.3f}" if row.get("scored") else "HDR: unscored"
            lines.append(f"| {row['scene']} | {row['mode']} | {row['bits']} | {row['video_bytes']} | {mean} | {p5} |")
    mpath = args.output / "mixed-results.json"
    if mpath.exists():
        lines += ["", "## Sustained CPU load with simultaneous GPU jobs", "", "CPU load always uses native-resolution SVT preset 4 / CRF26 / 10 bit.", "", "| CPU jobs | GPU jobs | GPU mode | GPU bits | GPU fps median | CPU fps median | Runs |", "|---:|---:|---|---:|---:|---:|---:|"]
        groups = {}
        for row in json.loads(mpath.read_text()):
            groups.setdefault((row["cpu_jobs"], row["gpu_jobs"], row["mode"], row["bits"]), []).append(row)
        for key, values in groups.items():
            lines.append("| " + " | ".join(map(str, key)) + f" | {statistics.median(v['gpu_fps'] for v in values):.2f} | {statistics.median(v['cpu_fps'] for v in values):.2f} | {len(values)} |")
    (args.output / "report.md").write_text("\n".join(lines) + "\n")
    print("REPORT", args.output / "report.md", flush=True)


def main():
    parser = argparse.ArgumentParser(description=__doc__)
    parser.add_argument("--ffmpeg", required=True)
    parser.add_argument("--ffprobe", required=True)
    parser.add_argument("--manifest", type=Path, required=True)
    parser.add_argument("--output", type=Path, required=True)
    parser.add_argument("--phase", choices=["quality", "decode", "pipelines", "audit", "speed", "mixed", "report", "all"], default="all")
    parser.add_argument("--quality-frames", type=int, default=360)
    parser.add_argument("--speed-frames", type=int, default=2400)
    parser.add_argument("--repeats", type=int, default=3)
    parser.add_argument("--scenes", help="Comma-separated scene ids")
    parser.add_argument("--gpu-modes", help="Limit speed sweep to comma-separated GPU modes")
    parser.add_argument("--cpu-modes", help="Limit speed sweep to comma-separated CPU modes")
    parser.add_argument("--bits", default="10,8", help="Output depths for speed/mixed phases")
    args = parser.parse_args()
    args.output = args.output.resolve()
    args.output.mkdir(parents=True, exist_ok=True)
    args.ffmpeg = str(Path(args.ffmpeg).resolve(strict=True))
    args.ffprobe = str(Path(args.ffprobe).resolve(strict=True))
    args.bits = [int(value) for value in args.bits.split(",")]
    if not args.bits or any(value not in (8, 10) for value in args.bits):
        parser.error("Output depths must be 8 or 10")
    if args.gpu_modes and any(mode not in MODES for mode in args.gpu_modes.split(",")):
        parser.error("Unknown GPU mode")
    if args.cpu_modes and any(mode not in {"cpu-scale", "cpu-native"} for mode in args.cpu_modes.split(",")):
        parser.error("CPU modes must be cpu-scale or cpu-native")
    if min(args.quality_frames, args.speed_frames, args.repeats) < 1:
        parser.error("Frame counts and repeats must be positive")
    scenes = json.loads(args.manifest.read_text())
    for scene in scenes:
        scene["source"] = str(Path(scene["source"]).resolve(strict=True))
        scene["stream"] = next(s for s in lab.probe(scene["source"], args.ffprobe)["streams"] if s["codec_type"] == "video")
        if scene["stream"]["codec_name"] not in {"h264", "hevc", "av1", "vp9"}:
            raise ValueError("Decode timings require original compressed inputs, not FFV1/raw references")
        scene["hdr"] = scene["stream"].get("color_transfer") in {"smpte2084", "arib-std-b67"}
        if not scene["id"].replace("-", "").replace("_", "").isalnum():
            raise ValueError("Scene ids must be safe filename components")
    if args.scenes:
        scenes = [s for s in scenes if s["id"] in args.scenes.split(",")]
    if not scenes:
        raise ValueError("No scenes selected")
    identity = {"ffmpeg_sha256": sha(args.ffmpeg), "ffprobe_sha256": sha(args.ffprobe),
                "manifest_sha256": sha(args.manifest), "quality_frames": args.quality_frames,
                "speed_frames": args.speed_frames, "repeats": args.repeats,
                "sources": [{"id": s["id"], "sha256": sha(s["source"])} for s in scenes]}
    if args.bits != [10, 8] or args.cpu_modes or args.gpu_modes:
        identity["sweep_selection"] = {"bits": args.bits, "cpu_modes": args.cpu_modes, "gpu_modes": args.gpu_modes}
    identity_path = args.output / "identity.json"
    if identity_path.exists() and json.loads(identity_path.read_text()) != identity:
        raise ValueError("Inputs/settings changed; use a fresh output directory")
    save(identity_path, identity)
    if not (args.output / "environment.json").exists():
        save(args.output / "environment.json", {"ffmpeg": subprocess.check_output([args.ffmpeg, "-version"], text=True),
                                               "cpu": Path("/proc/cpuinfo").read_text(),
                                               "memory": Path("/proc/meminfo").read_text(),
                                               "gpu": gpu(), "scenes": scenes, "runner_sha256": sha(__file__)})
    phases = ["decode", "quality", "pipelines", "audit", "speed", "mixed"] if args.phase == "all" else [args.phase]
    for phase in phases:
        {"decode": decode_check, "quality": quality, "pipelines": pipeline_quality, "audit": audit, "speed": speed, "mixed": contention,
         "report": lambda a, _: report(a)}[phase](args, scenes)
        report(args)


if __name__ == "__main__":
    main()

#!/usr/bin/env python3
"""Run isolated complete-film worker jobs, record resources, then score all frames.

The JSON manifest contains title, source, optional slug and existing_gpu paths.
Outputs and raw records stay in --output, outside versioned reports. Sources
are read only; the E2E harness uses disposable hard links and retains outputs.
"""

import argparse
import datetime as dt
import http.client
import json
import math
import os
from pathlib import Path
import re
import socket
import subprocess
import time

from av1_full_film_report import worst_window_p5


class DockerConnection(http.client.HTTPConnection):
    def connect(self):
        self.sock = socket.socket(socket.AF_UNIX, socket.SOCK_STREAM)
        self.sock.settimeout(4)
        self.sock.connect('/var/run/docker.sock')


def docker_json(path):
    connection = DockerConnection('localhost', timeout=4)
    try:
        connection.request('GET', path)
        response = connection.getresponse()
        data = response.read()
        if response.status != 200:
            raise RuntimeError(f'Docker status {response.status}')
        return json.loads(data)
    finally:
        connection.close()


def write_json(path, value):
    temporary = path.with_suffix(path.suffix + '.tmp')
    temporary.write_text(json.dumps(value, indent=2) + '\n')
    temporary.replace(path)


def stamp():
    return dt.datetime.now(dt.timezone.utc).isoformat()


def capture_encode_command(directory, container_id):
    destination = directory / 'ffmpeg-argv.json'
    if destination.exists():
        return
    processes = docker_json(f'/containers/{container_id}/top?ps_args=-o%20pid,comm')
    titles = [title.upper() for title in processes['Titles']]
    pid_column = titles.index('PID')
    for process in processes['Processes']:
        pid = int(process[pid_column])
        try:
            arguments = [part.decode() for part in Path(f'/proc/{pid}/cmdline').read_bytes().split(b'\0') if part]
        except (OSError, UnicodeDecodeError):
            continue
        if arguments and Path(arguments[0]).name == 'ffmpeg' and '-c:v' in arguments:
            if any(codec in arguments for codec in ('libsvtav1', 'av1_nvenc')):
                write_json(destination, arguments)
                return


def source_identity(source):
    info = source.stat()
    return dict(path=str(source), size=info.st_size, mtime_ns=info.st_mtime_ns)


def run_worker(args, film, profile):
    directory = args.output / film['slug'] / profile
    directory.mkdir(parents=True, exist_ok=True)
    output = directory / 'movie.mkv'
    record = directory / 'run.json'
    settings = dict(source=source_identity(Path(film['source'])), profile=profile,
                    cpu_crf=args.cpu_crf, server_image=args.server_image,
                    worker_image=args.worker_image, image_ids=args.image_ids)
    if record.exists():
        cached = json.loads(record.read_text())
        if cached['settings'] != settings:
            raise RuntimeError(f'Cached run settings differ: {record}')
        if (cached['returncode'] == 0 and cached.get('resource_samples', 0) > 0
                and output.exists() and output.stat().st_size == cached['output_bytes']
                and (directory / 'job.json').exists()):
            print(f"Retaining completed {film['title']} {profile}", flush=True)
            return
        if not args.retry_incomplete:
            raise RuntimeError(f'Previous run failed; inspect before --retry-incomplete: {record}')
    if output.exists() or (directory / 'worker.json').exists() or record.exists():
        if not args.retry_incomplete:
            raise RuntimeError(f'Incomplete prior run at {directory}; inspect before --retry-incomplete')
        archive = directory.with_name(directory.name + '.incomplete-' + dt.datetime.now(dt.timezone.utc).strftime('%Y%m%d-%H%M%S'))
        directory.rename(archive)
        directory.mkdir()
    environment = os.environ.copy()
    environment['PATH'] = str(Path(args.go).parent) + os.pathsep + environment.get('PATH', '')
    environment.update(E2E_GPU='1' if profile == 'gpu' else '0',
                       E2E_MOVIE_PATH=film['source'], E2E_WORK_DIR=str(directory),
                       E2E_KEEP_OUTPUT=str(output), E2E_REPORT_DIR=str(directory),
                       E2E_SERVER_IMAGE=args.server_image, E2E_WORKER_IMAGE=args.worker_image,
                       E2E_CPU_PRESET=profile.removeprefix('cpu-p'),
                       E2E_CPU_CRF=str(args.cpu_crf), E2E_JOB_TIMEOUT='6h')
    command = [args.go, 'test', '-count=1', '-tags=integration', '-v',
               '-timeout=7h', '-run=^TestDockerE2E$', './integration']
    write_json(directory / 'launch.json', dict(settings=settings, command=command, started_at=stamp()))
    print(f"Starting full worker job: {film['title']} {profile}", flush=True)
    start = time.monotonic()
    samples = 0
    with (directory / 'e2e.log').open('w') as log, (directory / 'resources.jsonl').open('w') as telemetry:
        process = subprocess.Popen(command, cwd=args.repo, env=environment,
                                   stdout=log, stderr=subprocess.STDOUT)
        last_notice = start
        while process.poll() is None:
            metadata = directory / 'worker.json'
            if metadata.exists():
                container_id = json.loads(metadata.read_text())['container_id']
                try:
                    stats = docker_json(f'/containers/{container_id}/stats?stream=false&one-shot=true')
                    row = dict(at=stamp(), cpu=stats['cpu_stats'], memory=stats['memory_stats'],
                               pids=stats.get('pids_stats', {}))
                    telemetry.write(json.dumps(row) + '\n')
                    telemetry.flush()
                    samples += 1
                    if samples % 5 == 0:
                        capture_encode_command(directory, container_id)
                except (OSError, RuntimeError, KeyError, TimeoutError):
                    pass  # Startup/cleanup can race the last stats request.
            if time.monotonic() - last_notice >= 60:
                print(f"Running {film['title']} {profile}: {(time.monotonic()-start)/60:.1f} min, {samples} resource samples", flush=True)
                last_notice = time.monotonic()
            time.sleep(1)
    result = dict(settings=settings, returncode=process.returncode,
                  e2e_wall_seconds=time.monotonic()-start, finished_at=stamp(),
                  resource_samples=samples, output_bytes=output.stat().st_size if output.exists() else None)
    write_json(record, result)
    if process.returncode or not samples:
        raise RuntimeError(f'Full-film job failed or resource telemetry absent: {directory}')
    print(f"Completed {film['title']} {profile}: {result['output_bytes']} bytes", flush=True)


def probe(args, source, directory):
    path = directory / 'decoded-probe.json'
    identity = source_identity(source)
    if path.exists():
        saved = json.loads(path.read_text())
        if saved['source'] != identity:
            raise RuntimeError(f'Changed source for probe: {source}')
        return saved
    command = [args.ffprobe, '-v', 'error', '-threads', '12', '-select_streams', 'v:0', '-count_frames',
               '-show_entries', 'stream=codec_name,width,height,pix_fmt,r_frame_rate,nb_read_frames:format=duration,size',
               '-of', 'json', str(source)]
    with (directory / 'probe-errors.log').open('w') as errors:
        data = json.loads(subprocess.check_output(command, stderr=errors, text=True))
    # Some valid H.264 sources contain truncated optional SEI metadata. FFprobe
    # reports it at error log level but still decodes every frame successfully;
    # preserve those diagnostics without treating them as damaged video.
    diagnostics = (directory / 'probe-errors.log').read_text().splitlines()
    fatal = [line for line in diagnostics if not ('SEI type 46' in line and 'truncated at' in line)]
    if fatal:
        raise RuntimeError(f'Source/output decode reported errors: {directory / "probe-errors.log"}')
    packet_command = [args.ffprobe, '-v', 'error', '-select_streams', 'v:0',
                      '-show_packets', '-show_entries', 'packet=size', '-of', 'csv=p=0', str(source)]
    with (directory / 'packet-errors.log').open('w') as errors:
        packets = subprocess.Popen(packet_command, stdout=subprocess.PIPE, stderr=errors, text=True)
        video_bytes = 0
        for line in packets.stdout:
            size = line.strip().split(',')[0]
            if size.isdecimal():
                video_bytes += int(size)
        packet_returncode = packets.wait()
        packet_diagnostics = (directory / 'packet-errors.log').read_text().splitlines()
        fatal_packet_diagnostics = [line for line in packet_diagnostics
                                    if not ('SEI type 46' in line and 'truncated at' in line)]
        if packet_returncode or fatal_packet_diagnostics:
            raise RuntimeError(f'Packet probe failed: {source}')
    result = dict(source=identity, probe=data, video_bytes=video_bytes,
                  frames=int(data['streams'][0]['nb_read_frames']),
                  commands=[command, packet_command])
    write_json(path, result)
    return result


def score(args, film, profile, source_probe):
    directory = args.output / film['slug'] / profile
    directory.mkdir(parents=True, exist_ok=True)
    encoded = Path(film['existing_gpu']) if profile == 'gpu' and film.get('existing_gpu') else directory / 'movie.mkv'
    output_probe = probe(args, encoded, directory)
    if output_probe['frames'] != source_probe['frames']:
        raise RuntimeError(f"Decoded frame counts differ: {film['title']} {profile}")
    scores = directory / 'vmaf-frames.json'
    summary = directory / 'quality.json'
    identity = dict(reference=source_probe['source'], distorted=output_probe['source'])
    if summary.exists():
        cached = json.loads(summary.read_text())
        if cached['inputs'] != identity or cached.get('analysis_version') != 2:
            raise RuntimeError(f'Quality cache inputs changed: {summary}')
        return
    if scores.exists():
        raise RuntimeError(f'Incomplete VMAF output must be inspected before retry: {scores}')
    fps = source_probe['probe']['streams'][0]['r_frame_rate']
    numerator, denominator = fps.split('/')
    stream = source_probe['probe']['streams'][0]
    width = int(stream['width'])
    height = int(stream['height'])
    target_width = min(1920, width)
    target_height = max(2, round(height * target_width / width))
    target_height -= target_height % 2
    expected_frames = source_probe['frames']
    source_fifo = directory / 'vmaf-reference.yuv.fifo'
    distorted_fifo = directory / 'vmaf-distorted.yuv.fifo'
    for fifo in (source_fifo, distorted_fifo):
        if fifo.exists():
            fifo.unlink()
        os.mkfifo(fifo)
    vf = f'scale={target_width}:{target_height},format=yuv420p'
    common = [args.ffmpeg, '-y', '-hide_banner', '-nostdin', '-v', 'error', '-xerror',
              '-r', fps]
    reference_command = common + ['-i', film['source'], '-frames:v', str(expected_frames),
                                  '-vf', vf, '-pix_fmt', 'yuv420p', '-fps_mode', 'passthrough',
                                  '-f', 'rawvideo', str(source_fifo)]
    distorted_command = common + ['-i', str(encoded), '-frames:v', str(expected_frames),
                                  '-vf', vf, '-pix_fmt', 'yuv420p', '-fps_mode', 'passthrough',
                                  '-f', 'rawvideo', str(distorted_fifo)]
    command = [args.vmaf_cli, '--reference', str(source_fifo), '--distorted', str(distorted_fifo),
               '--width', str(target_width), '--height', str(target_height), '--pixel_format', '420',
               '--bitdepth', '8', '--model', 'version=vmaf_v0.6.1', '--threads', str(args.vmaf_threads),
               '--frame_cnt', str(expected_frames), '--output', str(scores), '--json']
    print(f"Scoring all {expected_frames} frames: {film['title']} {profile}", flush=True)
    write_json(directory / 'vmaf-command.json', dict(reference_ffmpeg=reference_command,
                                                     distorted_ffmpeg=distorted_command,
                                                     vmaf=command))
    start = time.monotonic()
    writers = []
    try:
        writers.append(subprocess.Popen(reference_command, stdout=subprocess.DEVNULL,
                                        stderr=(directory / 'vmaf-reference-decode.log').open('w')))
        writers.append(subprocess.Popen(distorted_command, stdout=subprocess.DEVNULL,
                                        stderr=(directory / 'vmaf-distorted-decode.log').open('w')))
        with (directory / 'vmaf.log').open('w') as log:
            result = subprocess.run(command, stdout=log, stderr=subprocess.STDOUT, check=False)
        writer_codes = [writer.wait(timeout=60) for writer in writers]
        if result.returncode or any(code != 0 for code in writer_codes):
            raise RuntimeError(f'VMAF failed: cli={result.returncode}, ffmpeg={writer_codes}; see {directory}')
    finally:
        for writer in writers:
            if writer.poll() is None:
                writer.kill()
                writer.wait()
        for fifo in (source_fifo, distorted_fifo):
            if fifo.exists():
                fifo.unlink()
    data = json.loads(scores.read_text())
    values = [frame['metrics']['vmaf'] for frame in data['frames']]
    if len(values) != expected_frames or any(frame['frameNum'] != index for index, frame in enumerate(data['frames'])):
        raise RuntimeError(f'VMAF omitted or repeated frames: {scores}')
    if not all(math.isfinite(value) for value in values):
        raise RuntimeError(f'VMAF contains invalid scores: {scores}')
    ordered = sorted(values)
    percentile = lambda p: ordered[max(0, math.ceil(len(ordered)*p)-1)]
    minute = round(60 * int(numerator)/int(denominator))
    minute_p5 = []
    for index in range(0, len(values), minute):
        window = sorted(values[index:index+minute])
        if len(window) >= minute // 2:
            minute_p5.append(dict(start_frame=index, frames=len(window), p5=window[math.ceil(len(window)*.05)-1]))
    result = dict(analysis_version=2, metric_model='vmaf_v0.6.1', pixel_format='yuv420p',
                  inputs=identity, frames=len(values), mean=sum(values)/len(values),
                  p5=percentile(.05), p1=percentile(.01), minimum=min(values),
                  worst_minute=worst_window_p5(values, minute),
                  minute_p5=minute_p5, vmaf_wall_seconds=time.monotonic()-start,
                  output_bytes=encoded.stat().st_size, video_bytes=output_probe['video_bytes'])
    write_json(summary, result)
    print(f"Scored {film['title']} {profile}: p5={result['p5']:.3f}, mean={result['mean']:.3f}", flush=True)


def main():
    parser = argparse.ArgumentParser(description=__doc__)
    parser.add_argument('--manifest', type=Path, required=True)
    parser.add_argument('--output', type=Path, required=True)
    parser.add_argument('--repo', type=Path, default=Path(__file__).resolve().parents[1])
    parser.add_argument('--go', required=True)
    parser.add_argument('--ffmpeg', required=True)
    parser.add_argument('--ffprobe', default='/usr/bin/ffprobe')
    parser.add_argument('--vmaf-cli', default='vmaf', help='Standalone Netflix libvmaf CLI')
    parser.add_argument('--vmaf-threads', type=int, default=os.cpu_count() or 1)
    parser.add_argument('--server-image', default='ghcr.io/segator/transcoderd:server-codex-av1-worker-profiles')
    parser.add_argument('--worker-image', default='ghcr.io/segator/transcoderd:worker-codex-av1-worker-profiles')
    parser.add_argument('--cpu-crf', type=int, default=23)
    parser.add_argument('--profiles', default='cpu-p6,cpu-p4,cpu-p3,gpu')
    parser.add_argument('--retry-incomplete', action='store_true', help='Archive reviewed incomplete/failed runs before retrying')
    parser.add_argument('--phase', choices=('encode', 'quality', 'all'), default='all')
    args = parser.parse_args()
    args.output = args.output.resolve()
    args.output.mkdir(parents=True, exist_ok=True)
    profiles = args.profiles.split(',')
    if not profiles or any(profile != 'gpu' and not re.fullmatch(r'cpu-p(?:[0-9]|1[0-3])', profile) for profile in profiles):
        parser.error('profiles must be gpu or cpu-p0 through cpu-p13')
    if not 1 <= args.cpu_crf <= 63:
        parser.error('cpu-crf must be 1 through 63')
    image_lines = subprocess.check_output(['docker', 'image', 'inspect', args.server_image,
                                           args.worker_image, '--format', '{{json .Id}}'], text=True)
    args.image_ids = [json.loads(line) for line in image_lines.splitlines()]
    films = json.loads(args.manifest.read_text())
    for index, film in enumerate(films):
        film.setdefault('slug', f'film-{index+1}')
        if not re.fullmatch(r'[A-Za-z0-9][A-Za-z0-9_.-]*', film['slug']):
            raise RuntimeError('Film slugs must be plain directory names')
        if not Path(film['source']).is_file():
            raise RuntimeError(f"Missing source: {film['source']}")
    if args.phase in ('encode', 'all'):
        for film in films:
            for profile in profiles:
                if profile == 'gpu' and film.get('existing_gpu'):
                    continue
                run_worker(args, film, profile)
    if args.phase in ('quality', 'all'):
        for film in films:
            directory = args.output / film['slug'] / 'source'
            directory.mkdir(parents=True, exist_ok=True)
            source_probe = probe(args, Path(film['source']), directory)
            for profile in profiles:
                score(args, film, profile, source_probe)


if __name__ == '__main__':
    main()

#!/usr/bin/env python3
"""Time the same production CPU/GPU profiles on native Windows and WSL.

Run from WSL. Keep input/output on each OS's own filesystem. Outputs, full
arguments, hashes and native process timing are retained for offline validation.
This measures an excerpt, not a full-film job or bare-metal Linux performance.
"""

import argparse
import hashlib
import json
import os
from pathlib import Path
import re
import resource
import subprocess
import time


def sha256(path):
    with path.open('rb') as stream:
        return hashlib.file_digest(stream, 'sha256').hexdigest()


def wslpath(mode, path):
    return subprocess.check_output(['wslpath', mode, str(path)], text=True).strip()


def arguments(source, output, codec, duration):
    result = ['-hide_banner', '-nostdin', '-nostats', '-benchmark', '-fflags', '+genpts',
              '-threads', '0', '-analyzeduration', '2147483647', '-probesize', '2147483647',
              '-i', str(source), '-t', str(duration), '-map', '0:v:0', '-map', '0:a:0',
              '-map', '0:a:1', '-map_chapters', '-1', '-map_metadata', '-1',
              '-max_muxing_queue_size', '9999', '-avoid_negative_ts', 'make_zero', '-copyts',
              '-flags', '+global_header', '-filter:v',
              "scale='min(1920,iw)':-1:force_original_aspect_ratio=decrease"]
    if codec == 'cpu':
        result += ['-pix_fmt', 'yuv420p10le', '-c:v', 'libsvtav1', '-preset', '4', '-crf', '25']
    else:
        result += ['-pix_fmt', 'p010le', '-c:v', 'av1_nvenc', '-preset', 'p7', '-tune', 'uhq',
                   '-rc', 'vbr', '-cq', '38', '-b:v', '0', '-multipass', 'fullres', '-bf', '4',
                   '-b_ref_mode', 'middle', '-split_encode_mode', 'disabled', '-qmin', '0', '-qmax', '100']
    return result + ['-c:a', 'libfdk_aac', '-vbr', '5', '-metadata:s:a:0', 'language=spa',
                     '-metadata:s:a:1', 'language=eng', '-max_interleave_delta', '0', '-y', str(output)]


def main():
    parser = argparse.ArgumentParser(description=__doc__)
    parser.add_argument('--linux-ffmpeg', type=Path, required=True)
    parser.add_argument('--windows-ffmpeg', required=True)
    parser.add_argument('--source', type=Path, required=True)
    parser.add_argument('--windows-source', required=True)
    parser.add_argument('--windows-output', required=True)
    parser.add_argument('--output', type=Path, required=True)
    parser.add_argument('--idle-container')
    parser.add_argument('--duration', type=int, default=120)
    parser.add_argument('--repeats', type=int, default=3)
    args = parser.parse_args()
    args.output.mkdir(parents=True, exist_ok=True)
    if (args.output / 'results.json').exists():
        raise RuntimeError('Use a fresh output directory; timing evidence is not overwritten')
    windows_output = Path(wslpath('-u', args.windows_output))
    windows_output.mkdir(parents=True, exist_ok=True)
    source_hash = sha256(args.source)
    if sha256(Path(wslpath('-u', args.windows_source))) != source_hash:
        raise RuntimeError('Windows and Linux source bytes differ')
    environment = os.environ.copy()
    environment['LD_LIBRARY_PATH'] = '/usr/lib/wsl/lib:' + environment.get('LD_LIBRARY_PATH', '')
    script = Path(__file__).resolve().parent / 'windows/measure-encode.ps1'
    results = {'source_sha256': source_hash, 'duration': args.duration, 'runs': []}
    (args.output / 'settings.json').write_text(json.dumps(vars(args), default=str, indent=2))

    def require_idle():
        if args.idle_container:
            status = subprocess.check_output(['docker', 'exec', args.idle_container,
                                              '/app/transcoderd-worker', 'control', 'status'], text=True)
            if not re.search(r'\bactive=0\b', status) or not re.search(r'\bencodes=0/', status):
                raise RuntimeError('Production work overlaps the timing batch: ' + status.strip())
        return time.time()

    def run(platform, codec, index, duration, warmup=False):
        idle_at = require_idle()
        name = f'{platform}-{codec}-' + ('warmup' if warmup else str(index))
        root = args.output if platform == 'linux' else windows_output
        output = root / (name + '.mkv')
        source = args.source if platform == 'linux' else args.windows_source
        output_arg = str(output) if platform == 'linux' else wslpath('-w', output)
        argv = arguments(source, output_arg, codec, duration)
        (args.output / (name + '-argv.json')).write_text(json.dumps(argv, indent=2))
        print(f'Starting {name}', flush=True)
        if platform == 'linux':
            before = resource.getrusage(resource.RUSAGE_CHILDREN)
            start = time.perf_counter()
            with (root / (name + '.stderr')).open('wb') as log:
                process = subprocess.run([str(args.linux_ffmpeg), *argv], env=environment,
                                         stdout=log, stderr=subprocess.STDOUT, check=True)
            wall = time.perf_counter() - start
            after = resource.getrusage(resource.RUSAGE_CHILDREN)
            measurement = dict(exit_code=process.returncode, wall_seconds=wall,
                               cpu_seconds=after.ru_utime + after.ru_stime - before.ru_utime - before.ru_stime)
        else:
            argument_file = root / (name + '-argv.json')
            argument_file.write_text(json.dumps(argv), encoding='utf-8')
            result_file = root / (name + '.json')
            subprocess.run(['powershell.exe', '-NoProfile', '-NonInteractive', '-ExecutionPolicy', 'Bypass',
                            '-File', wslpath('-w', script), '-Executable', args.windows_ffmpeg,
                            '-ArgumentsFile', wslpath('-w', argument_file), '-ResultFile', wslpath('-w', result_file)],
                           check=True)
            measurement = json.loads(result_file.read_text(encoding='utf-8-sig'))
        measurement.update(platform=platform, codec=codec, repetition=index, warmup=warmup,
                           output=str(output), output_bytes=output.stat().st_size, idle_checked_at=idle_at)
        results['runs'].append(measurement)
        (args.output / 'results.json').write_text(json.dumps(results, indent=2))
        print(f'Completed {name}: {measurement["wall_seconds"]:.3f}s, {measurement["output_bytes"]} bytes', flush=True)
        time.sleep(3)

    for platform, codec in [('linux', 'cpu'), ('windows', 'cpu'), ('linux', 'gpu'), ('windows', 'gpu')]:
        run(platform, codec, 0, 5, warmup=True)
    for repetition in range(1, args.repeats + 1):
        order = [('linux', 'cpu'), ('windows', 'cpu'), ('linux', 'gpu'), ('windows', 'gpu')]
        if repetition % 2 == 0:
            order.reverse()
        for platform, codec in order:
            run(platform, codec, repetition, args.duration)


if __name__ == '__main__':
    main()

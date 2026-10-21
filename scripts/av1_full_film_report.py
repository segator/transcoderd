#!/usr/bin/env python3
"""Summarize completed full-film E2E, Docker resource and every-frame VMAF records."""

import argparse
from bisect import bisect_left, insort
import csv
import datetime as dt
import json
import math
from pathlib import Path


def worst_window_p5(values, window):
    if not values or window < 1:
        raise ValueError('A nonempty sequence and positive window are required')
    window = min(window, len(values))
    ordered = sorted(values[:window])
    rank = math.ceil(window*.05)-1
    result = dict(start_frame=0, frames=window, p5=ordered[rank])
    for index in range(window, len(values)):
        ordered.pop(bisect_left(ordered, values[index-window]))
        insort(ordered, values[index])
        if ordered[rank] < result['p5']:
            result = dict(start_frame=index-window+1, frames=window, p5=ordered[rank])
    return result


def seconds(value):
    return dt.datetime.fromisoformat(value.replace('Z', '+00:00')).timestamp()


def phases(events):
    result = {}
    for event in events:
        phase = event['notificationType']
        if event['status'] == 'started':
            result.setdefault(phase, {})['start'] = seconds(event['eventTime'])
        elif event['status'] == 'completed':
            result.setdefault(phase, {})['end'] = seconds(event['eventTime'])
    for value in result.values():
        if 'start' in value and 'end' in value:
            value['seconds'] = value['end'] - value['start']
    return result


def resource_summary(rows, start, end):
    if end <= start:
        raise ValueError('Invalid phase interval')
    rows = sorted(rows, key=lambda row: seconds(row['at']))
    times = [seconds(row['at']) for row in rows]
    if not times or times[0] > start or times[-1] < end:
        raise ValueError('Resource samples do not bracket the complete phase')

    def counter_at(at):
        index = bisect_left(times, at)
        if times[index] == at:
            return rows[index]['cpu']['cpu_usage']['total_usage']
        left, right = rows[index-1], rows[index]
        fraction = (at-times[index-1])/(times[index]-times[index-1])
        a = left['cpu']['cpu_usage']['total_usage']
        b = right['cpu']['cpu_usage']['total_usage']
        return a + (b-a)*fraction

    core_seconds = (counter_at(end)-counter_at(start))/1e9
    available = rows[bisect_left(times, start)]['cpu']['online_cpus']
    cpu_peaks = []
    working_set = []
    anonymous = []
    samples = 0
    for index, row in enumerate(rows):
        if start <= times[index] <= end:
            samples += 1
            memory = row['memory']
            stats = memory.get('stats', {})
            inactive = stats.get('inactive_file', stats.get('total_inactive_file', 0))
            working_set.append(max(0, memory['usage']-inactive))
            anonymous.append(stats.get('anon', stats.get('rss', 0)))
        if index and times[index] > start and times[index-1] < end:
            before = rows[index-1]
            elapsed = times[index]-times[index-1]
            delta = row['cpu']['cpu_usage']['total_usage']-before['cpu']['cpu_usage']['total_usage']
            if elapsed > 0:
                cpu_peaks.append(delta/1e9/elapsed*100)
    if not samples:
        raise ValueError('No resource samples within phase')
    return dict(cpu_core_seconds=core_seconds, average_logical_cpus=core_seconds/(end-start),
                average_cpu_percent=core_seconds/(end-start)*100,
                peak_sample_cpu_percent=max(cpu_peaks), available_logical_cpus=available,
                average_capacity_percent=core_seconds/(end-start)/available*100,
                peak_working_memory_bytes=max(working_set), peak_anonymous_memory_bytes=max(anonymous),
                samples_in_phase=samples,
                max_sample_gap_seconds=max(times[index]-times[index-1] for index in range(1, len(times))
                                           if times[index] > start and times[index-1] < end))


def collect(root, films):
    results, pending = [], []
    for film in films:
        source_file = root / film['slug'] / 'source' / 'decoded-probe.json'
        for profile in ('gpu', 'cpu-p6', 'cpu-p4', 'cpu-p3'):
            directory = root / film['slug'] / profile
            required = [source_file, directory/'run.json', directory/'job.json', directory/'quality.json']
            if not all(path.exists() for path in required):
                pending.append(dict(title=film['title'], profile=profile))
                continue
            run = json.loads((directory/'run.json').read_text())
            if run['returncode'] != 0:
                raise ValueError(f'Failed worker job: {directory}')
            job = json.loads((directory/'job.json').read_text())
            quality = json.loads((directory/'quality.json').read_text())
            source = json.loads(source_file.read_text())
            timing = phases(job['events'])
            samples = [json.loads(line) for line in (directory/'resources.jsonl').read_text().splitlines()]
            resources = resource_summary(samples, timing['FFMPEG']['start'], timing['FFMPEG']['end'])
            row = dict(title=film['title'], profile=profile,
                       quality_setting=34 if profile == 'gpu' else run['settings']['cpu_crf'],
                       source_bytes=source['source']['size'], output_bytes=quality['output_bytes'],
                       source_video_bytes=source['video_bytes'], video_bytes=quality['video_bytes'],
                       full_file_saving_percent=(1-quality['output_bytes']/source['source']['size'])*100,
                       video_saving_percent=(1-quality['video_bytes']/source['video_bytes'])*100,
                       source_minutes=float(source['probe']['format']['duration'])/60,
                       ffmpeg_phase_minutes=timing['FFMPEG']['seconds']/60,
                       worker_job_minutes=timing['Job']['seconds']/60,
                       e2e_minutes=run['e2e_wall_seconds']/60,
                       vmaf_analysis_minutes=quality['vmaf_wall_seconds']/60,
                       vmaf_mean=quality['mean'], vmaf_p5=quality['p5'], vmaf_p1=quality['p1'],
                       worst_minute_p5=quality['worst_minute']['p5'],
                       worst_minute_start_frame=quality['worst_minute']['start_frame'],
                       scored_frames=quality['frames'], **resources)
            results.append(row)
    return dict(results=results, pending=pending)


def main():
    parser = argparse.ArgumentParser(description=__doc__)
    parser.add_argument('--root', type=Path, required=True)
    parser.add_argument('--manifest', type=Path, required=True)
    args = parser.parse_args()
    report = collect(args.root, json.loads(args.manifest.read_text()))
    (args.root/'summary.json').write_text(json.dumps(report, indent=2)+'\n')
    if report['results']:
        with (args.root/'summary.csv').open('w') as output:
            writer = csv.DictWriter(output, fieldnames=report['results'][0].keys())
            writer.writeheader()
            writer.writerows(report['results'])
    print(json.dumps(report, indent=2))


if __name__ == '__main__':
    main()

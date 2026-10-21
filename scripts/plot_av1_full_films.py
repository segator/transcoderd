#!/usr/bin/env python3
"""Plot completed full-film measurements without extrapolating short scenes."""

import argparse
import json
from pathlib import Path

import matplotlib
matplotlib.use('Agg')
import matplotlib.pyplot as plt


PROFILES = ('gpu', 'cpu-p6', 'cpu-p4', 'cpu-p3')
LABELS = ('GPU p7/UHQ', 'CPU preset 6', 'CPU preset 4', 'CPU preset 3')
COLORS = ('#13836c', '#2676c8', '#8b5bc4', '#d47816')


def bar(axis, values, title, formatter):
    axis.barh(range(4), values, color=COLORS)
    axis.set_yticks(range(4), LABELS)
    axis.invert_yaxis()
    axis.set_title(title, fontsize=11, loc='left')
    axis.set_xlim(0, max(values)*1.3 if max(values) else 1)
    axis.grid(axis='x', alpha=.18)
    axis.set_axisbelow(True)
    for index, value in enumerate(values):
        axis.text(value+max(values)*.025, index, formatter(value), va='center', fontsize=9)
    axis.spines[['top', 'right']].set_visible(False)


def minutes(value):
    total = round(value*60)
    return f'{total//60}m {total%60:02d}s'


def main():
    parser = argparse.ArgumentParser(description=__doc__)
    parser.add_argument('--root', type=Path, required=True)
    parser.add_argument('--manifest', type=Path, required=True)
    args = parser.parse_args()
    report = json.loads((args.root/'summary.json').read_text())
    if report['pending']:
        raise RuntimeError('Complete the planned jobs and every-frame scoring before plotting the final comparison')
    films = json.loads(args.manifest.read_text())
    rows = {(row['title'], row['profile']): row for row in report['results']}
    plt.rcParams.update({'font.family': 'DejaVu Sans', 'font.size': 10,
                         'figure.facecolor': 'white', 'axes.facecolor': 'white'})
    figure, axes = plt.subplots(len(films), 3, figsize=(17, 3.8*len(films)), squeeze=False)
    for index, film in enumerate(films):
        points = [rows[(film['title'], profile)] for profile in PROFILES]
        bar(axes[index, 0], [p['ffmpeg_phase_minutes'] for p in points],
            film['title']+'\nEncode phase (minutes)', minutes)
        bar(axes[index, 1], [p['output_bytes']/1e9 for p in points],
            'Complete MKV, including AAC (GB)', lambda value: f'{value:.2f} GB')
        quality = axes[index, 2]
        for position, (point, color) in enumerate(zip(points, COLORS)):
            quality.plot([point['worst_minute_p5'], point['vmaf_p5']], [position, position], color=color, alpha=.45)
            quality.scatter(point['vmaf_p5'], position, color=color, s=65, zorder=3)
            quality.scatter(point['worst_minute_p5'], position, color=color, marker='x', s=65, zorder=3)
            quality.annotate(f"{point['vmaf_p5']:.2f}", (point['vmaf_p5'], position), xytext=(0, -16), textcoords='offset points', ha='center', fontsize=8)
            quality.annotate(f"{point['worst_minute_p5']:.2f}", (point['worst_minute_p5'], position), xytext=(0, 8), textcoords='offset points', ha='center', fontsize=8)
        quality.set_yticks(range(4), LABELS)
        quality.set_ylim(3.6, -.6)
        quality.set_xlim(max(0, min(p['worst_minute_p5'] for p in points)-4), 101)
        quality.axvspan(91, 92, color='#cfd4da', alpha=.55)
        quality.grid(axis='x', alpha=.18)
        quality.set_title('p5 VMAF: ● whole film / × worst rolling minute', fontsize=11, loc='left')
        quality.spines[['top', 'right']].set_visible(False)
    figure.suptitle('Complete-film AV1 results — measured files and times', fontsize=17, y=1.01)
    figure.text(.5, -.025, 'One job at a time. GPU CQ34; CPU CRF23. Quality is measured, not forced equal. Sizes are decimal GB.', ha='center', fontsize=10)
    figure.tight_layout(w_pad=3, h_pad=3)
    figure.savefig(args.root/'full-film-comparison.png', dpi=170, bbox_inches='tight')
    plt.close(figure)

    figure, axes = plt.subplots(len(films), 1, figsize=(16, 3.8*len(films)), squeeze=False)
    for index, film in enumerate(films):
        axis = axes[index, 0]
        source = json.loads((args.root/film['slug']/'source/decoded-probe.json').read_text())
        numerator, denominator = source['probe']['streams'][0]['r_frame_rate'].split('/')
        fps = int(numerator)/int(denominator)
        floor = 100
        for profile, label, color in zip(PROFILES, LABELS, COLORS):
            quality = json.loads((args.root/film['slug']/profile/'quality.json').read_text())
            blocks = quality['minute_p5']
            x = [point['start_frame']/fps/60 for point in blocks]
            y = [point['p5'] for point in blocks]
            floor = min(floor, min(y))
            axis.plot(x, y, color=color, label=label, linewidth=1.25)
        axis.axhspan(91, 92, color='#cfd4da', alpha=.5, label='91–92 target band')
        axis.set_title(film['title'], loc='left')
        axis.set_xlabel('Movie minute')
        axis.set_ylabel('p5 within each one-minute block')
        axis.set_ylim(max(0, min(90, floor-2)), 100.5)
        axis.grid(alpha=.18)
        axis.spines[['top', 'right']].set_visible(False)
        if index == 0:
            axis.legend(ncol=5, loc='lower left', fontsize=9)
    figure.suptitle('Quality across the complete film — every frame scored', fontsize=17, y=1.01)
    figure.tight_layout(h_pad=2)
    figure.savefig(args.root/'full-film-quality-timeline.png', dpi=170, bbox_inches='tight')
    plt.close(figure)

    figure, axes = plt.subplots(len(films), 2, figsize=(14, 3.5*len(films)), squeeze=False)
    for index, film in enumerate(films):
        points = [rows[(film['title'], profile)] for profile in PROFILES]
        bar(axes[index, 0], [p['average_capacity_percent'] for p in points],
            film['title']+'\nAverage CPU share of available logical CPUs (%)', lambda value: f'{value:.1f}%')
        bar(axes[index, 1], [p['peak_working_memory_bytes']/1e9 for p in points],
            'Peak container working memory (GB)', lambda value: f'{value:.2f} GB')
    figure.suptitle('One worker, one encoding job — container resources', fontsize=17, y=1.01)
    figure.text(.5, -.025, 'Encode-phase samples. Memory excludes inactive file cache. CPU use is not a measurement of electricity consumption.', ha='center', fontsize=10)
    figure.tight_layout(h_pad=3, w_pad=3)
    figure.savefig(args.root/'full-film-resources.png', dpi=170, bbox_inches='tight')


if __name__ == '__main__':
    main()

import unittest

import av1_full_film_report as report


class ResourceSummaryTest(unittest.TestCase):
    def sample(self, at, core_seconds, memory=10_000):
        return dict(at=f'2026-10-20T10:00:{at:02d}+00:00',
                    cpu=dict(cpu_usage=dict(total_usage=core_seconds*1e9), online_cpus=24),
                    memory=dict(usage=memory, stats=dict(inactive_file=2000, anon=6000)))

    def test_cpu_capacity_and_memory_exclude_idle_phases(self):
        samples = [self.sample(0, 0), self.sample(10, 10), self.sample(20, 190), self.sample(30, 200)]
        result = report.resource_summary(samples, report.seconds('2026-10-20T10:00:10Z'),
                                          report.seconds('2026-10-20T10:00:20Z'))
        self.assertAlmostEqual(result['average_logical_cpus'], 18)
        self.assertAlmostEqual(result['average_cpu_percent'], 1800)
        self.assertAlmostEqual(result['average_capacity_percent'], 75)
        self.assertEqual(result['peak_working_memory_bytes'], 8000)
        self.assertEqual(result['peak_anonymous_memory_bytes'], 6000)

    def test_interpolates_irregular_boundary_samples(self):
        samples = [self.sample(0, 0), self.sample(10, 100), self.sample(20, 300)]
        result = report.resource_summary(samples, report.seconds('2026-10-20T10:00:05Z'),
                                          report.seconds('2026-10-20T10:00:15Z'))
        self.assertAlmostEqual(result['cpu_core_seconds'], 150)
        self.assertAlmostEqual(result['average_logical_cpus'], 15)

    def test_rejects_incomplete_telemetry(self):
        with self.assertRaises(ValueError):
            report.resource_summary([self.sample(10, 10), self.sample(20, 20)],
                                    report.seconds('2026-10-20T10:00:05Z'),
                                    report.seconds('2026-10-20T10:00:15Z'))


class RollingQualityTest(unittest.TestCase):
    def test_detects_bad_frames_across_fixed_minute_boundary(self):
        values = [100]*120
        values[58:62] = [0]*4
        self.assertEqual(sorted(values[:60])[2], 100)
        self.assertEqual(sorted(values[60:])[2], 100)
        self.assertEqual(report.worst_window_p5(values, 60)['p5'], 0)

    def test_short_clip_uses_all_available_frames(self):
        result = report.worst_window_p5([90, 95, 100], 60)
        self.assertEqual(result, dict(start_frame=0, frames=3, p5=90))


if __name__ == '__main__':
    unittest.main()

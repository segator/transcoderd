import math
import unittest

import av1_lab


class AV1LabTests(unittest.TestCase):
    def test_windows_cover_start_middle_and_end_without_overrun(self):
        self.assertEqual(av1_lab.windows(30, 60, 3), [(0.0, 30)])
        samples = av1_lab.windows(120, 20, 3)
        self.assertEqual(samples, [(15.0, 20), (50.0, 20), (85.0, 20)])
        self.assertTrue(all(start + length <= 120 for start, length in samples))

    def test_interpolation_matches_quality_without_extrapolation(self):
        points = [
            {"vmaf_mean": 94.0, "vmaf_p5": 80.0, "video_bytes": 1_000_000, "encode_seconds": [5, 7, 100]},
            {"vmaf_mean": 98.0, "vmaf_p5": 90.0, "video_bytes": 4_000_000, "encode_seconds": [9, 11, 100]},
        ]
        self.assertIsNone(av1_lab.interpolate(points, 93.0))
        self.assertIsNone(av1_lab.interpolate(points, 99.0))
        matched = av1_lab.interpolate(points, 96.0)
        self.assertTrue(math.isclose(matched["video_bytes"], 2_000_000))
        self.assertEqual(matched["encode_seconds"], 9)
        self.assertEqual(matched["vmaf_p5"], 85)
        self.assertEqual(av1_lab.interpolate(points[:1], 94.0)["video_bytes"], 1_000_000)

    def test_source_classes_keep_unmeasured_hdr_separate(self):
        source = {"codec": "hevc", "height": 2160, "bit_depth": 10, "hdr": False}
        self.assertEqual(av1_lab.category(source), "hevc/4k+/10bit/sdr")
        source["hdr"] = True
        self.assertEqual(av1_lab.category(source), "hevc/4k+/10bit/hdr")


if __name__ == "__main__":
    unittest.main()

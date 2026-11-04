import importlib.util
import json
import math
from pathlib import Path
import tempfile
import unittest

import av1_svt_versions as svt

spec = importlib.util.spec_from_file_location("svt_toolchain", Path(__file__).parent / "ffmpeg/svt-toolchain.py")
toolchain = importlib.util.module_from_spec(spec)
spec.loader.exec_module(toolchain)


class SVTVersionsTests(unittest.TestCase):
    def test_replacing_binary_or_reference_invalidates_cache(self):
        with tempfile.TemporaryDirectory() as directory:
            binary = Path(directory) / "ffmpeg"
            reference = Path(directory) / "reference.mkv"
            binary.write_bytes(b"old executable")
            reference.write_bytes(b"old pixels")

            def key():
                return svt.cache_key({"binary": svt.fingerprint(binary)},
                                     [{"sha256": svt.sha256(reference)}], [25], 3, False)

            cache = Path(directory) / "identity.json"
            original = key()
            cache.write_text(json.dumps({"key": original}))
            svt.check_cache(cache, original)
            for changed in (binary, reference):
                with self.subTest(changed=changed.name):
                    previous = changed.read_bytes()
                    changed.write_bytes(b"replaced at identical path")
                    with self.assertRaises(ValueError):
                        svt.check_cache(cache, key())
                    changed.write_bytes(previous)

    def test_identical_binary_content_is_not_a_version_comparison(self):
        with self.assertRaises(ValueError):
            svt.check_versions({"path": "/a", "sha256": "same"}, {"path": "/b", "sha256": "same"})

    def test_frame_pairing_rejects_truncation_and_duplicate_timestamps(self):
        for output in ([0, .04], [0, .04, .04], [0, .04, .12]):
            with self.subTest(output=output), self.assertRaises(ValueError):
                svt.check_timeline([0, .04, .08], output)

    def test_quality_rejects_missing_indices_nonfinite_and_wrong_counts(self):
        for frames in (
            [],
            [{"frameNum": 0, "metrics": {"vmaf": 90}}],
            [{"frameNum": i, "metrics": {"vmaf": 90}} for i in (0, 0)],
            [{"frameNum": i, "metrics": {"vmaf": math.nan}} for i in (0, 1)],
        ):
            with self.subTest(frames=frames), self.assertRaises(ValueError):
                svt.quality_summary({"frames": frames}, 2, 24)

    def test_percentiles_do_not_destroy_temporal_order(self):
        values = [99, 98, 40, 41, 97, 96]
        data = {"frames": [{"frameNum": i, "metrics": {"vmaf": v}} for i, v in enumerate(values)]}
        result = svt.quality_summary(data, 6, 2)
        self.assertEqual(result["worst_1s_start"], 2)
        self.assertEqual(result["worst_1s_mean"], 40.5)
        self.assertEqual(result["vmaf_p5"], 40)
        self.assertEqual(result["worst_frame"], 2)
        self.assertEqual(data["frames"][0]["metrics"]["vmaf"], 99)

    def test_dependency_override_preserves_other_pins_and_fails_on_drift(self):
        old = f'VER_SVTAV1=("4.2.0" "{toolchain.HASHES["4.2.0"]}")'
        surrounding = 'VER_FFMPEG=("9.0.2" "hash")\n'
        result = toolchain.override(surrounding + old + "\n", "4.3.0")
        self.assertTrue(result.startswith(surrounding))
        self.assertIn(toolchain.HASHES["4.3.0"], result)
        for text in ("missing", old + "\n" + old):
            with self.subTest(text=text), self.assertRaises(ValueError):
                toolchain.override(text, "4.3.0")


if __name__ == "__main__":
    unittest.main()

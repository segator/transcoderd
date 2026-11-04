# Verification

The failed job was windows-native in run 41734285792 (PR #202), which reported missing capture files for both TestFFMPEGExecutionUsesPinnedProfileForEveryOption cases. The Unix shebang fixture did not execute as Windows FFmpeg. This is an added test's portability bug, not a production encoder failure.

- Linux targeted regression with race detector: both CPU/GPU cases passed.
- Linux full worker/step package with race detector and coverage: passed.
- Windows amd64 test binary executed natively on the Windows host through WSL interop: both targeted cases passed, including the exact exit status 23 and preservation of source paths with spaces and Unicode. Full worker/step package also passed.
- Linux golangci-lint v2.11.4: 0 issues. Strict OpenSpec validation passed with skip_specs, since runtime behavior is unchanged.
- windows-unit now includes worker/step as an early regression gate; full windows-native and Windows E2E gates remain enabled.

GitHub Actions run [41791497546](https://github.com/segator/transcoderd/actions/runs/41791497546), commit 0b50ef8, passes windows-unit, pwa-tests and ci (including Linux E2E). The windows-unit log explicitly reports both pinned-profile regression cases passing. Lint and all CodeQL analyses also pass for this commit. The native desktop dashboard JavaScript test passes locally.

At archive time the full windows-build job is still running and windows-native has not started. Their outcome is pending; neither full Windows bundle validation nor Windows E2E is claimed here. Archiving records the completed test implementation and native regression validation; complete-pipeline delivery validation remains in progress on PR #202.

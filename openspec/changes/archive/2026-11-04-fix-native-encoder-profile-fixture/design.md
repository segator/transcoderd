# Design

Use TestMain to recognize a narrowly named fixture environment variable before testing flag parsing. The child serializes os.Args[1:] as JSON and exits 23; fixture I/O failures exit differently. The normal parent process always runs m.Run. Copy the already-built native test executable to a temporary PATH directory as ffmpeg (ffmpeg.exe on Windows), including a directory with spaces. Each case gets its own capture path.

This works for precompiled Windows test binaries without Go, sh, PowerShell scripts, external FFmpeg or GPU hardware. Preserve the actual FFMPEGStepExecutor command path and check the wrapped exec.ExitError code so a missing or wrong executable cannot masquerade as the intended fixture failure. Verify Linux/race, native Windows execution through the available host, cross-platform lint, and the required GitHub checks. No specification or runtime change is needed.

Include worker/step in the early windows-unit job while keeping the complete windows-native media and E2E checks intact.

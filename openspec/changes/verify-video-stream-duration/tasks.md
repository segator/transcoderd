# Tasks

## 1. Packet timeline probing

- [x] 1.1 Add a streaming FFprobe packet-timeline duration probe and unit tests for out-of-order timestamps, missing PTS, malformed rows, and empty output.

## 2. Duration verification

- [x] 2.1 Use packet timelines only when container duration comparison exceeds the configured tolerance; test subtitle-extended valid output, real truncation, scan failure, and unchanged size verification.
- [x] 2.2 Run the focused Go package tests and strict OpenSpec validation.

# Tasks

- [x] Pin a recent FFmpeg source build with software and NVIDIA AV1 encoders.
- [x] Wire a versioned builder image through Makefile and CI without replacing the previous tag.
- [x] Build the builder and worker images locally; inspect encoders and smoke-test software AV1 encoding.
- [x] Pass the RTX 5080 into the worker container and verify a decodable 10-bit `av1_nvenc` output and CUDA decode/scale path.
- [x] Run relevant Go tests and `openspec validate --all --strict`.
- [x] Sync the distribution contract and archive this completed change.

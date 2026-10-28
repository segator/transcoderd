# Changelog

## [2.9.0](https://github.com/segator/transcoderd/compare/v2.8.1...v2.9.0) (2026-10-28)


### 🎉 Features

* **dashboard:** share worker UI across platforms ([#191](https://github.com/segator/transcoderd/issues/191)) ([148b945](https://github.com/segator/transcoderd/commit/148b94552f2d26d372dddb2fccfc0686a1c8dbc6))
* **windows:** enforce signed unattended installation updates ([#193](https://github.com/segator/transcoderd/issues/193)) ([0fdef3f](https://github.com/segator/transcoderd/commit/0fdef3f677bf2f3e4e0bc23896fe6e97ea54c5da))

## [2.8.1](https://github.com/segator/transcoderd/compare/v2.8.0...v2.8.1) (2026-10-27)


### 🔧 Miscellaneous Chores

* **deps:** bump github.com/Microsoft/go-winio from 0.6.2 to 0.6.4 ([#189](https://github.com/segator/transcoderd/issues/189)) ([c3fb3fc](https://github.com/segator/transcoderd/commit/c3fb3fc872587ec36c004f74223f63c4c25ea347))

## [2.8.0](https://github.com/segator/transcoderd/compare/v2.7.3...v2.8.0) (2026-10-26)


### 🎉 Features

* **worker:** add native Windows bundle and CPU E2E CI ([#187](https://github.com/segator/transcoderd/issues/187)) ([12dbf82](https://github.com/segator/transcoderd/commit/12dbf821b6e5755905680ebb354a3479c4b0f8d6))
* **worker:** persist runtime job limits ([#185](https://github.com/segator/transcoderd/issues/185)) ([9af8c94](https://github.com/segator/transcoderd/commit/9af8c9480b3ed81775c129c5adbe3f0b0fdd96e7))
* **worker:** prefetch pipeline jobs for encoders ([#186](https://github.com/segator/transcoderd/issues/186)) ([4dd0706](https://github.com/segator/transcoderd/commit/4dd070678054bc941be1858f12aea14299445586))


### 📝 Documentation

* sync upload staging cleanup spec ([#183](https://github.com/segator/transcoderd/issues/183)) ([ae6f4ef](https://github.com/segator/transcoderd/commit/ae6f4efdc4b1ba22e414ad19278b7284518f32b6))

## [2.7.3](https://github.com/segator/transcoderd/compare/v2.7.2...v2.7.3) (2026-10-25)


### 🐛 Bug Fixes

* remove empty upload staging directories ([#181](https://github.com/segator/transcoderd/issues/181)) ([0d7b3b7](https://github.com/segator/transcoderd/commit/0d7b3b759c935329bcf73035082d8a75319dcbf6))

## [2.7.2](https://github.com/segator/transcoderd/compare/v2.7.1...v2.7.2) (2026-10-25)


### 🐛 Bug Fixes

* **update:** keep workers available when GitHub is rate limited ([#179](https://github.com/segator/transcoderd/issues/179)) ([aaa9990](https://github.com/segator/transcoderd/commit/aaa999048e4b295fbd66807437209c75ecb6098b))

## [2.7.1](https://github.com/segator/transcoderd/compare/v2.7.0...v2.7.1) (2026-10-25)


### 🐛 Bug Fixes

* **update:** target canonical executable during self-update ([#177](https://github.com/segator/transcoderd/issues/177)) ([9fa3d82](https://github.com/segator/transcoderd/commit/9fa3d824e35e158e501389199440e8515656b9d4))

## [2.7.0](https://github.com/segator/transcoderd/compare/v2.6.1...v2.7.0) (2026-10-25)


### 🎉 Features

* **worker:** allow weekday selectors in schedule shortcut ([#175](https://github.com/segator/transcoderd/issues/175)) ([5562695](https://github.com/segator/transcoderd/commit/55626955b26c7894ee32b84bf41bb0c5545f8bec))


### 🐛 Bug Fixes

* **worker:** resize encode concurrency at runtime ([#176](https://github.com/segator/transcoderd/issues/176)) ([ed5284b](https://github.com/segator/transcoderd/commit/ed5284b373f4ac9d1a0464f26a8be79a9dbb8400))
* **worker:** skip terminal contexts during resume ([#173](https://github.com/segator/transcoderd/issues/173)) ([dbc889f](https://github.com/segator/transcoderd/commit/dbc889f661392e5348a7c6b4792cf163957a1e62))


### 📝 Documentation

* **openspec:** archive PGS bitmap overflow fix ([#167](https://github.com/segator/transcoderd/issues/167)) ([90a4567](https://github.com/segator/transcoderd/commit/90a4567c941840d9b9f7701bed5c11014ab28d17))

## [2.6.1](https://github.com/segator/transcoderd/compare/v2.6.0...v2.6.1) (2026-10-25)


### 🐛 Bug Fixes

* **worker:** verify video stream duration on mismatch ([#171](https://github.com/segator/transcoderd/issues/171)) ([01ee596](https://github.com/segator/transcoderd/commit/01ee5960c6ded7575041bd0a89a7f90f1fc4f37a))

## [2.6.0](https://github.com/segator/transcoderd/compare/v2.5.8...v2.6.0) (2026-10-24)


### 🎉 Features

* **worker:** tune AV1 quality defaults ([#169](https://github.com/segator/transcoderd/pull/169)) ([1d26758](https://github.com/segator/transcoderd/commit/1d267589a5589f5c9e4e2342e97a607713158b6f))
* **worker:** add runtime job admission controls ([#163](https://github.com/segator/transcoderd/issues/163)) ([25957f3](https://github.com/segator/transcoderd/commit/25957f33ee99f87a16338ebaef36caa4c3f8f2da))


### 🐛 Bug Fixes

* **server:** make completed outputs readable over SMB ([#168](https://github.com/segator/transcoderd/issues/168)) ([8a63848](https://github.com/segator/transcoderd/commit/8a63848f8a3fabe93a2a0d0a69380fed02383811))
* **subtitles:** guard PGS bitmap pixel bounds ([#166](https://github.com/segator/transcoderd/issues/166)) ([58eb54b](https://github.com/segator/transcoderd/commit/58eb54b667309cc5219c5279407036b9b9359afc))
* **update:** drain active jobs before self-update ([#165](https://github.com/segator/transcoderd/issues/165)) ([8f18761](https://github.com/segator/transcoderd/commit/8f18761cef152fc3c267f585c7702c16177f5b4e))


### 📝 Documentation

* **openspec:** archive PGS OCR concurrency change ([#161](https://github.com/segator/transcoderd/issues/161)) ([9ae8186](https://github.com/segator/transcoderd/commit/9ae818629f60432322f57fcf0e49c5e66f33ebe1))
* **openspec:** remove archived admission delta ([#164](https://github.com/segator/transcoderd/issues/164)) ([b6922ea](https://github.com/segator/transcoderd/commit/b6922ea52b3e51fb685b4c795524d9489922b69f))

## [2.5.8](https://github.com/segator/transcoderd/compare/v2.5.7...v2.5.8) (2026-10-24)


### 🐛 Bug Fixes

* **worker:** limit default PGS OCR concurrency ([#159](https://github.com/segator/transcoderd/issues/159)) ([0ef213e](https://github.com/segator/transcoderd/commit/0ef213e114027a1d1a02bf4f5a6e57d8de615f4d))

## [2.5.7](https://github.com/segator/transcoderd/compare/v2.5.6...v2.5.7) (2026-10-24)


### 🐛 Bug Fixes

* **update:** shut down child cleanly on cancellation ([#156](https://github.com/segator/transcoderd/issues/156)) ([f438427](https://github.com/segator/transcoderd/commit/f4384272b7c7ba0c88c14f23244755438ff99608))
* **worker:** convert mov_text subtitles directly to SRT ([#155](https://github.com/segator/transcoderd/issues/155)) ([07ddecf](https://github.com/segator/transcoderd/commit/07ddecf981b92c93d14907ee208181545c7ae997))
* **worker:** redraw active progress trackers ([#158](https://github.com/segator/transcoderd/issues/158)) ([191fc41](https://github.com/segator/transcoderd/commit/191fc41372be92a150a996b5036c3a1352c94c34))


### 📝 Documentation

* **openspec:** archive AV1 output naming change ([#153](https://github.com/segator/transcoderd/issues/153)) ([c632e82](https://github.com/segator/transcoderd/commit/c632e823eed264c9167b94d74733f2c3632ae5fd))


### 🔧 Miscellaneous Chores

* ignore local media files ([#157](https://github.com/segator/transcoderd/issues/157)) ([250bf60](https://github.com/segator/transcoderd/commit/250bf607f08ee95385d6aa7d17b3cafbc6755d62))

## [2.5.6](https://github.com/segator/transcoderd/compare/v2.5.5...v2.5.6) (2026-10-23)


### 🐛 Bug Fixes

* **server:** isolate upload retry staging ([#151](https://github.com/segator/transcoderd/issues/151)) ([1e0dd77](https://github.com/segator/transcoderd/commit/1e0dd7782403968e9d96d65845eb47b7a6a5c55e))

## [2.5.5](https://github.com/segator/transcoderd/compare/v2.5.4...v2.5.5) (2026-10-23)


### 🐛 Bug Fixes

* **server:** finalize uploads across filesystems ([#149](https://github.com/segator/transcoderd/issues/149)) ([2701158](https://github.com/segator/transcoderd/commit/2701158a734a8a993f0cd4ef40b41ae21aeb4e26))

## [2.5.4](https://github.com/segator/transcoderd/compare/v2.5.3...v2.5.4) (2026-10-23)


### 🐛 Bug Fixes

* **server:** name outputs from probed video codec ([#143](https://github.com/segator/transcoderd/issues/143)) ([65190b1](https://github.com/segator/transcoderd/commit/65190b1cd73782996df47e15152f398b8bddc644))


### ⚡ Performance Improvements

* **ci:** improve Dagger cache reuse ([#147](https://github.com/segator/transcoderd/issues/147)) ([e69fe69](https://github.com/segator/transcoderd/commit/e69fe69bb71fba0e02184f63ee547c1220ce67b7))


### 🤖 Continuous Integration

* make Dagger own the build pipeline ([#145](https://github.com/segator/transcoderd/issues/145)) ([ee1867d](https://github.com/segator/transcoderd/commit/ee1867debcfd48d41fe6d6a15393b8b8dbb2c698))
* preserve E2E package install cache ([#146](https://github.com/segator/transcoderd/issues/146)) ([81986fc](https://github.com/segator/transcoderd/commit/81986fc31deb8a372abe4499631096c798e1b16e))
* simplify pipeline and install Mage binary ([#148](https://github.com/segator/transcoderd/issues/148)) ([ab1d308](https://github.com/segator/transcoderd/commit/ab1d308dfc8cfd64f532f5211e29d929bba2bc33))

## [2.5.3](https://github.com/segator/transcoderd/compare/v2.5.2...v2.5.3) (2026-10-23)


### 🐛 Bug Fixes

* **ci:** install GitHub CLI on release runner ([#141](https://github.com/segator/transcoderd/issues/141)) ([5f0b2af](https://github.com/segator/transcoderd/commit/5f0b2af48a8d55971b91025f8d68e55e73a77096))

## [2.5.2](https://github.com/segator/transcoderd/compare/v2.5.1...v2.5.2) (2026-10-23)


### 🐛 Bug Fixes

* publish release aliases from canonical image ([#140](https://github.com/segator/transcoderd/issues/140)) ([2e76688](https://github.com/segator/transcoderd/commit/2e76688b57379c768dd5b55fe76714c26e69d628))
* **security:** distrust compromised eMudhra certificates ([#136](https://github.com/segator/transcoderd/issues/136)) ([df7b08e](https://github.com/segator/transcoderd/commit/df7b08e08d1592dcba60db14392ebc80a39bdc14))


### 🤖 Continuous Integration

* prepare Transcoderd workflows for K3s ARC ([#138](https://github.com/segator/transcoderd/issues/138)) ([086f946](https://github.com/segator/transcoderd/commit/086f94671c5250b8e9bb754911b5e94de8384c38))


### 🔧 Miscellaneous Chores

* **deps:** bump github.com/sirupsen/logrus from 1.9.4 to 1.10.2 ([#139](https://github.com/segator/transcoderd/issues/139)) ([f1d7184](https://github.com/segator/transcoderd/commit/f1d7184d1a774542197365ffe4bdc2deba976e57))

## [2.5.1](https://github.com/segator/transcoderd/compare/v2.5.0...v2.5.1) (2026-10-22)


### 🐛 Bug Fixes

* **ci:** use workflow token for Dependabot merges ([#134](https://github.com/segator/transcoderd/issues/134)) ([f774e99](https://github.com/segator/transcoderd/commit/f774e990e63a0db0143b6e86e98b3e5983d95749))


### 🔧 Miscellaneous Chores

* **deps:** bump github.com/asticode/go-astisub from 0.41.0 to 0.45.0 ([#129](https://github.com/segator/transcoderd/issues/129)) ([5e04dc2](https://github.com/segator/transcoderd/commit/5e04dc21813ab2adb30cbfb0806adde6daeafdb4))
* **deps:** bump github.com/jedib0t/go-pretty/v6 from 6.8.1 to 6.8.3 ([#132](https://github.com/segator/transcoderd/issues/132)) ([00967e7](https://github.com/segator/transcoderd/commit/00967e7080c43d9475663242e1e24097a2c0738c))
* **deps:** bump gopkg.in/vansante/go-ffprobe.v2 from 2.3.0 to 2.3.1 ([#131](https://github.com/segator/transcoderd/issues/131)) ([538b60b](https://github.com/segator/transcoderd/commit/538b60b6606d533fbe74618a955d521f8b432978))

## [2.5.0](https://github.com/segator/transcoderd/compare/v2.4.3...v2.5.0) (2026-10-22)


### 🎉 Features

* migrate build system from Makefile to Mage + Dagger ([#112](https://github.com/segator/transcoderd/issues/112)) ([404974e](https://github.com/segator/transcoderd/commit/404974e49316ef67bf5ebe08980fe74fee5a73b7))
* **worker:** use approved AV1 profiles with bounded encode jobs ([#126](https://github.com/segator/transcoderd/issues/126)) ([c1e1a5d](https://github.com/segator/transcoderd/commit/c1e1a5ddbb91254c413ecf3e1ada22952ac83356))


### 🐛 Bug Fixes

* restore subtitle metadata stream specifiers to preserve multi-language audio ([#111](https://github.com/segator/transcoderd/issues/111)) ([ffaa7cb](https://github.com/segator/transcoderd/commit/ffaa7cbe46eb2f9291f97513afe051f548720a92))


### 📝 Documentation

* add OpenSpec baseline and align agent guidance ([#122](https://github.com/segator/transcoderd/issues/122)) ([bc884c5](https://github.com/segator/transcoderd/commit/bc884c5b8dc0625d569c5e8d12ca139314669d50))


### 🏗️ Build System

* package FFmpeg 9.0.2 with AV1 encoders ([#123](https://github.com/segator/transcoderd/issues/123)) ([6615ab3](https://github.com/segator/transcoderd/commit/6615ab3993d7eeea709fbcf45ddfc0e519a9080d))


### 🔧 Miscellaneous Chores

* **deps:** bump github.com/asticode/go-astisub from 0.39.0 to 0.40.0 ([#110](https://github.com/segator/transcoderd/issues/110)) ([16e38fe](https://github.com/segator/transcoderd/commit/16e38fec25b928fe8f1bec27f25e1c2707561c76))
* **deps:** bump github.com/asticode/go-astisub from 0.40.0 to 0.41.0 ([#119](https://github.com/segator/transcoderd/issues/119)) ([e0ee8d3](https://github.com/segator/transcoderd/commit/e0ee8d3745e14e5e795e3d64899da4729698aa56))
* **deps:** bump github.com/jedib0t/go-pretty/v6 from 6.7.10 to 6.8.1 ([#116](https://github.com/segator/transcoderd/issues/116)) ([9b6184f](https://github.com/segator/transcoderd/commit/9b6184fb44b5b54875a292f1049b1bea2705f60a))
* **deps:** bump github.com/jedib0t/go-pretty/v6 from 6.7.9 to 6.7.10 ([#108](https://github.com/segator/transcoderd/issues/108)) ([9383c6f](https://github.com/segator/transcoderd/commit/9383c6fb9d3bd74b804bd63884fdc0ba9672cba2))
* **deps:** bump github.com/testcontainers/testcontainers-go from 0.42.0 to 0.43.0 ([#117](https://github.com/segator/transcoderd/issues/117)) ([601e77c](https://github.com/segator/transcoderd/commit/601e77c051ada644ce5391a1f5286ccbd87f350f))
* **deps:** bump github.com/testcontainers/testcontainers-go/modules/postgres from 0.42.0 to 0.43.0 ([#118](https://github.com/segator/transcoderd/issues/118)) ([97e7956](https://github.com/segator/transcoderd/commit/97e79562b6bc1d99b78837a32451f0d93ddb0f62))
* **deps:** bump gopkg.in/ini.v1 from 1.67.1 to 1.67.2 ([#113](https://github.com/segator/transcoderd/issues/113)) ([681ca91](https://github.com/segator/transcoderd/commit/681ca91bd20423b7d7c8d25c9f8c68f94921d221))
* **deps:** bump gopkg.in/ini.v1 from 1.67.2 to 1.67.3 ([#115](https://github.com/segator/transcoderd/issues/115)) ([cd6ddfc](https://github.com/segator/transcoderd/commit/cd6ddfcde92327154989de5ce8bbe74a0f4c7baf))
* **deps:** bump the go_modules group across 1 directory with 2 updates ([#121](https://github.com/segator/transcoderd/issues/121)) ([70f5bab](https://github.com/segator/transcoderd/commit/70f5bab322b5b68909e8a3f99ee7ccf167817d6c))
* **deps:** bump the go_modules group across 1 directory with 4 updates ([#127](https://github.com/segator/transcoderd/issues/127)) ([dda1e62](https://github.com/segator/transcoderd/commit/dda1e62ec462f25987f681e240d593705c6e48e5))


### ✅ Tests

* add AV1 library benchmark lab ([#124](https://github.com/segator/transcoderd/issues/124)) ([7ef1680](https://github.com/segator/transcoderd/commit/7ef1680747b6dfd8cb8686f04713a63e849fd1fe))

## [2.4.3](https://github.com/segator/transcoderd/compare/v2.4.2...v2.4.3) (2026-04-19)


### 🐛 Bug Fixes

* add Dutch (dut/nl → nld) to PGS Tesseract language mapping ([#102](https://github.com/segator/transcoderd/issues/102)) ([d8078a7](https://github.com/segator/transcoderd/commit/d8078a7f0a93bd2451e1cab21253c763b6a4b45c))
* correct FFmpeg stream specifier and disposition flags for subtitles ([#100](https://github.com/segator/transcoderd/issues/100)) ([b1c4f4c](https://github.com/segator/transcoderd/commit/b1c4f4c826193d244d496dfaf3ee60d18fc8db14))
* fallback to English when PGS subtitle has empty language tag ([#103](https://github.com/segator/transcoderd/issues/103)) ([c3c0201](https://github.com/segator/transcoderd/commit/c3c02012f132ead79f8591f5ccf888cf06a64d68))
* handle ffprobe 'inverted' field as integer instead of bool ([#104](https://github.com/segator/transcoderd/issues/104)) ([322131c](https://github.com/segator/transcoderd/commit/322131c09aa3db509541d471ab64038eb743d4ac))
* skip empty PGS subtitle tracks instead of failing ([#106](https://github.com/segator/transcoderd/issues/106)) ([0c1ee10](https://github.com/segator/transcoderd/commit/0c1ee10af1138c5ce4d4f717496c6c213c5264b0))


### ✅ Tests

* Docker-based end-to-end integration test ([#107](https://github.com/segator/transcoderd/issues/107)) ([7d0a324](https://github.com/segator/transcoderd/commit/7d0a3249f1acfea042bcb505b1a1419d55f3412f))

## [2.4.2](https://github.com/segator/transcoderd/compare/v2.4.1...v2.4.2) (2026-04-16)


### 🐛 Bug Fixes

* remove progress tracker on completion ([#97](https://github.com/segator/transcoderd/issues/97)) ([6d071ae](https://github.com/segator/transcoderd/commit/6d071aed8ad5f6ee889afced9c086b3f76e4c4cb))


### 🔧 Miscellaneous Chores

* **deps:** update Go to 1.25 and bump all dependencies ([#99](https://github.com/segator/transcoderd/issues/99)) ([e89627a](https://github.com/segator/transcoderd/commit/e89627aa336b4a4ad8e37343ab74a3a6b02d41aa))

## [2.4.1](https://github.com/segator/transcoderd/compare/v2.4.0...v2.4.1) (2026-04-12)


### 🐛 Bug Fixes

* remove completedJobMaintenance cronjob ([#93](https://github.com/segator/transcoderd/issues/93)) ([53b6975](https://github.com/segator/transcoderd/commit/53b6975237be3b24d7dbbc09226c1ea6e78d4285))
* separate Docker cache refs from image tags to fix cross-run cache misses ([#90](https://github.com/segator/transcoderd/issues/90)) ([b756e70](https://github.com/segator/transcoderd/commit/b756e701dc2942cab7ea77f95c766f784f5b316a))
* use pre-built Docker builder images instead of BuildKit cache ([#94](https://github.com/segator/transcoderd/issues/94)) ([ba96c6a](https://github.com/segator/transcoderd/commit/ba96c6a191023c4ea631d73c728d58998966e12a))

## [2.4.0](https://github.com/segator/transcoderd/compare/v2.3.1...v2.4.0) (2026-04-11)


### 🎉 Features

* WebVTT subtitle extraction, completed job re-queue, Docker caching, unit tests ([#86](https://github.com/segator/transcoderd/issues/86)) ([aa58a92](https://github.com/segator/transcoderd/commit/aa58a92b99d2a94e941897b79a667c40907f0add))


### 🐛 Bug Fixes

* resolve command EOF callback and race condition in RunWithContext ([#88](https://github.com/segator/transcoderd/issues/88)) ([c13bc42](https://github.com/segator/transcoderd/commit/c13bc42b5b69e33d33f2243fb21f0236103ad6b1))
* use registry cache mode=max for Docker build cache images ([#89](https://github.com/segator/transcoderd/issues/89)) ([4b383f1](https://github.com/segator/transcoderd/commit/4b383f13e13ef4e49af08bc93a0b99fbcd733d35))


### 🏗️ Build System

* **deps:** ffmpeg build script to v1.57 ([#68](https://github.com/segator/transcoderd/issues/68)) ([5e4e8ee](https://github.com/segator/transcoderd/commit/5e4e8ee9c0d34644a03969f74c18b62dc0fee4ac))


### 🤖 Continuous Integration

* fix golangci-lint ([#60](https://github.com/segator/transcoderd/issues/60)) ([55e4907](https://github.com/segator/transcoderd/commit/55e4907baa8ef5fe159f4de9cf058d886ad245ed))
* lint code ([#69](https://github.com/segator/transcoderd/issues/69)) ([1274a58](https://github.com/segator/transcoderd/commit/1274a58139f55e09f9cab797c20ab16f4bbd796a))


### 🔧 Miscellaneous Chores

* **deps:** bump github.com/asticode/go-astisub from 0.32.1 to 0.34.0 ([2544776](https://github.com/segator/transcoderd/commit/25447764a533888dc5d64ad18bc79b9e33d99654))
* **deps:** bump github.com/asticode/go-astisub from 0.32.1 to 0.34.0 ([aa2b2c3](https://github.com/segator/transcoderd/commit/aa2b2c35b36968954fbf6dc8b4ee3a92bc3d07e9))
* **deps:** bump github.com/asticode/go-astisub from 0.34.0 to 0.35.0 ([#53](https://github.com/segator/transcoderd/issues/53)) ([1253884](https://github.com/segator/transcoderd/commit/1253884014547e4edb841d8a3f81cd359dd82471))
* **deps:** bump github.com/asticode/go-astisub from 0.35.0 to 0.36.0 ([#57](https://github.com/segator/transcoderd/issues/57)) ([f5c3f33](https://github.com/segator/transcoderd/commit/f5c3f33c72684987e4cf9a2c597667ee92cbab7f))
* **deps:** bump github.com/asticode/go-astisub from 0.36.0 to 0.38.0 ([#59](https://github.com/segator/transcoderd/issues/59)) ([9ad914b](https://github.com/segator/transcoderd/commit/9ad914b839ed3ccbfb0b229c29b457400f6efb6d))
* **deps:** bump github.com/asticode/go-astisub from 0.38.0 to 0.39.0 ([#80](https://github.com/segator/transcoderd/issues/80)) ([1ab182e](https://github.com/segator/transcoderd/commit/1ab182ec0c3bc8be636e52753f7493e7032e8637))
* **deps:** bump github.com/hashicorp/go-retryablehttp from 0.7.7 to 0.7.8 ([#44](https://github.com/segator/transcoderd/issues/44)) ([5e4fbed](https://github.com/segator/transcoderd/commit/5e4fbede42d0cbd39c72a285042d89450c4b1b20))
* **deps:** bump github.com/jedib0t/go-pretty/v6 from 6.6.7 to 6.6.9 ([#58](https://github.com/segator/transcoderd/issues/58)) ([432c7ca](https://github.com/segator/transcoderd/commit/432c7ca1045110477ef6a091aa58916f48724698))
* **deps:** bump github.com/jedib0t/go-pretty/v6 from 6.6.9 to 6.7.0 ([#61](https://github.com/segator/transcoderd/issues/61)) ([bc46a71](https://github.com/segator/transcoderd/commit/bc46a71151bf3e55042f19e5e460dbf6473b11d6))
* **deps:** bump github.com/jedib0t/go-pretty/v6 from 6.7.0 to 6.7.1 ([#62](https://github.com/segator/transcoderd/issues/62)) ([0bb0f75](https://github.com/segator/transcoderd/commit/0bb0f753c9b707a7274cc5e477e6282e1ebab8f8))
* **deps:** bump github.com/jedib0t/go-pretty/v6 from 6.7.1 to 6.7.2 ([#64](https://github.com/segator/transcoderd/issues/64)) ([c3c2e5d](https://github.com/segator/transcoderd/commit/c3c2e5da8f5d275ea676bfc2450f726cc4fcc8dd))
* **deps:** bump github.com/jedib0t/go-pretty/v6 from 6.7.2 to 6.7.3 ([#65](https://github.com/segator/transcoderd/issues/65)) ([54f5a38](https://github.com/segator/transcoderd/commit/54f5a38d1915e2904148d64c3f339163e052ea68))
* **deps:** bump github.com/jedib0t/go-pretty/v6 from 6.7.3 to 6.7.5 ([#67](https://github.com/segator/transcoderd/issues/67)) ([794e672](https://github.com/segator/transcoderd/commit/794e672deb09d20d5f46ebcfc04965949c410a48))
* **deps:** bump github.com/jedib0t/go-pretty/v6 from 6.7.5 to 6.7.7 ([#72](https://github.com/segator/transcoderd/issues/72)) ([68f55c4](https://github.com/segator/transcoderd/commit/68f55c4857931d57ceec03f53db3a0674689878a))
* **deps:** bump github.com/jedib0t/go-pretty/v6 from 6.7.7 to 6.7.8 ([#73](https://github.com/segator/transcoderd/issues/73)) ([95a9b95](https://github.com/segator/transcoderd/commit/95a9b9551375e5718f096c970186359a2b6d8dc9))
* **deps:** bump github.com/jedib0t/go-pretty/v6 from 6.7.8 to 6.7.9 ([#84](https://github.com/segator/transcoderd/issues/84)) ([3855864](https://github.com/segator/transcoderd/commit/385586472185825796ee96e7e236acf32306a61e))
* **deps:** bump github.com/lib/pq from 1.10.9 to 1.12.2 ([#83](https://github.com/segator/transcoderd/issues/83)) ([5f9ad28](https://github.com/segator/transcoderd/commit/5f9ad28756e3a8053364a13d794d0bdd61c16457))
* **deps:** bump github.com/sirupsen/logrus from 1.9.3 to 1.9.4 ([#76](https://github.com/segator/transcoderd/issues/76)) ([2ea9e0e](https://github.com/segator/transcoderd/commit/2ea9e0e143931af1400a63252db90b38d7d7011f))
* **deps:** bump github.com/spf13/pflag from 1.0.6 to 1.0.9 ([#50](https://github.com/segator/transcoderd/issues/50)) ([5bf4090](https://github.com/segator/transcoderd/commit/5bf409052c1ce341d9a98bef9ec4f23ed94ce3bd))
* **deps:** bump github.com/spf13/pflag from 1.0.9 to 1.0.10 ([#51](https://github.com/segator/transcoderd/issues/51)) ([af05cab](https://github.com/segator/transcoderd/commit/af05cab672060e5d917c6682f3c25f093c9832bc))
* **deps:** bump github.com/spf13/viper from 1.19.0 to 1.20.1 ([#38](https://github.com/segator/transcoderd/issues/38)) ([726ec04](https://github.com/segator/transcoderd/commit/726ec043d812be6bc6b2750c3128e5f58eea1404))
* **deps:** bump github.com/spf13/viper from 1.20.1 to 1.21.0 ([#52](https://github.com/segator/transcoderd/issues/52)) ([dd5d440](https://github.com/segator/transcoderd/commit/dd5d440eaa4ce52c58dc3b93fd2234f2cce75ff1))
* **deps:** bump golang.org/x/crypto from 0.44.0 to 0.45.0 in the go_modules group across 1 directory ([#66](https://github.com/segator/transcoderd/issues/66)) ([5335747](https://github.com/segator/transcoderd/commit/5335747bd137195f9a9ac5a6742af5a4d3dc5f1c))
* **deps:** bump golang.org/x/net from 0.33.0 to 0.35.0 ([810896a](https://github.com/segator/transcoderd/commit/810896a1b01335d7259242d80d137e288c4b6ada))
* **deps:** bump golang.org/x/net from 0.33.0 to 0.35.0 ([ba26161](https://github.com/segator/transcoderd/commit/ba261610e05ace411767a0729bb746caf1aa39f2))
* **deps:** bump golang.org/x/net from 0.35.0 to 0.36.0 in the go_modules group ([#36](https://github.com/segator/transcoderd/issues/36)) ([1198e07](https://github.com/segator/transcoderd/commit/1198e07b0811e714ec78a2ca1ac1d4b6f8451f06))
* **deps:** bump golang.org/x/net from 0.35.0 to 0.37.0 ([#35](https://github.com/segator/transcoderd/issues/35)) ([535fbc0](https://github.com/segator/transcoderd/commit/535fbc047e158144a13e57c5fe49ffae46bc8d76))
* **deps:** bump golang.org/x/net from 0.37.0 to 0.42.0 ([#45](https://github.com/segator/transcoderd/issues/45)) ([04ca6b3](https://github.com/segator/transcoderd/commit/04ca6b31571f99127cba2a9a4d6466ca565459ec))
* **deps:** bump golang.org/x/net from 0.42.0 to 0.43.0 ([#48](https://github.com/segator/transcoderd/issues/48)) ([178b559](https://github.com/segator/transcoderd/commit/178b55943b0fb5828e43158977f58b89f405203b))
* **deps:** bump golang.org/x/net from 0.43.0 to 0.44.0 ([#54](https://github.com/segator/transcoderd/issues/54)) ([8a47f16](https://github.com/segator/transcoderd/commit/8a47f163edd0e66ae89f9faef4cd8db114a7c8b2))
* **deps:** bump golang.org/x/net from 0.44.0 to 0.45.0 ([#55](https://github.com/segator/transcoderd/issues/55)) ([985d9bf](https://github.com/segator/transcoderd/commit/985d9bfa5f92ea54b755f94be991b8ac8c0a769c))
* **deps:** bump golang.org/x/net from 0.45.0 to 0.46.0 ([#56](https://github.com/segator/transcoderd/issues/56)) ([fbdaf98](https://github.com/segator/transcoderd/commit/fbdaf9896b2ea657a93331c604245061052fdf7c))
* **deps:** bump golang.org/x/net from 0.46.0 to 0.47.0 ([#63](https://github.com/segator/transcoderd/issues/63)) ([9275963](https://github.com/segator/transcoderd/commit/9275963e2382df4c270143b121823fe1cb9fe384))
* **deps:** bump golang.org/x/net from 0.47.0 to 0.48.0 ([#71](https://github.com/segator/transcoderd/issues/71)) ([8c27b7a](https://github.com/segator/transcoderd/commit/8c27b7ab87fe98604cb8405ed4a8ff02a208ef60))
* **deps:** bump gopkg.in/ini.v1 from 1.67.0 to 1.67.1 ([#74](https://github.com/segator/transcoderd/issues/74)) ([7f41d84](https://github.com/segator/transcoderd/commit/7f41d847127e8d6e67f70aabfc3c794dfd61a18a))
* **deps:** bump gopkg.in/ini.v1 from 1.67.0 to 1.67.1 ([#87](https://github.com/segator/transcoderd/issues/87)) ([691c0bc](https://github.com/segator/transcoderd/commit/691c0bcc60231ef427d3f74b6a086d8cdf78829b))
* **deps:** bump gopkg.in/vansante/go-ffprobe.v2 from 2.2.1 to 2.3.0 ([#75](https://github.com/segator/transcoderd/issues/75)) ([87db585](https://github.com/segator/transcoderd/commit/87db585e2b730e072c467c1b2b41e8c3660ec048))


### ✅ Tests

* add unittest ([#70](https://github.com/segator/transcoderd/issues/70)) ([52fa4dc](https://github.com/segator/transcoderd/commit/52fa4dc33e0c920e4bd0683e229db721289c0380))

## [2.3.1](https://github.com/segator/transcoderd/compare/v2.3.0...v2.3.1) (2025-03-02)


### 🔧 Miscellaneous Chores

* **deps:** bump github.com/asticode/go-astisub from 0.32.0 to 0.32.1 ([#26](https://github.com/segator/transcoderd/issues/26)) ([fb7a24a](https://github.com/segator/transcoderd/commit/fb7a24a0607737a00088b7473da74bed92cd8517))
* **deps:** bump github.com/jedib0t/go-pretty/v6 from 6.6.5 to 6.6.7 ([#28](https://github.com/segator/transcoderd/issues/28)) ([844efa1](https://github.com/segator/transcoderd/commit/844efa16a9f8b7a82e6c9e722ff54840590bcc67))


### ♻️ Code Refactoring

* server now calculates public IP instead of worker sending it ([#29](https://github.com/segator/transcoderd/issues/29)) ([7ac096f](https://github.com/segator/transcoderd/commit/7ac096f881730cbefb753bb89e212f1abfd5f56b))

## [2.3.0](https://github.com/segator/transcoderd/compare/v2.2.1...v2.3.0) (2025-02-10)


### 🎉 Features

* if a job failed more than 10 times is marked as canceled ([032b406](https://github.com/segator/transcoderd/commit/032b40613ce2246517d966bbd0623df7e2ac2ba7))


### 🐛 Bug Fixes

* progressJobMaintenance was not cleaning properly job_progress table ([15747ba](https://github.com/segator/transcoderd/commit/15747ba4e62ed433a63909d128755724b2069c31))

## [2.2.1](https://github.com/segator/transcoderd/compare/v2.2.0...v2.2.1) (2025-02-08)


### 🐛 Bug Fixes

* pgs conversation fails because working path is not correct ([9cd4ec7](https://github.com/segator/transcoderd/commit/9cd4ec7d7b232cd4b16cc96ae9879fa13cce5f84))

## [2.2.0](https://github.com/segator/transcoderd/compare/v2.1.0...v2.2.0) (2025-02-07)


### 🎉 Features

* add romanian tesseract lang ([f3447da](https://github.com/segator/transcoderd/commit/f3447dafc34e796b82dd53c6d809132a0d058041))
* masive refactor for fun ([#22](https://github.com/segator/transcoderd/issues/22)) ([f3447da](https://github.com/segator/transcoderd/commit/f3447dafc34e796b82dd53c6d809132a0d058041))
* progress events are not send to job_events now new table job_progress ([f3447da](https://github.com/segator/transcoderd/commit/f3447dafc34e796b82dd53c6d809132a0d058041))
* **server:** logs prints timestamp ([864b19d](https://github.com/segator/transcoderd/commit/864b19d62e103f6dab4da0129bb407109ad6f8eb))
* tessdata support language zho ([7c23c9b](https://github.com/segator/transcoderd/commit/7c23c9b15ae5a0cd91bb447cf2c3f3a5e0e333d9))


### 🐛 Bug Fixes

* some minor fixes ([f3447da](https://github.com/segator/transcoderd/commit/f3447dafc34e796b82dd53c6d809132a0d058041))


### 🔧 Miscellaneous Chores

* change logger format for an specific log message regarding db conn usage ([0cbf476](https://github.com/segator/transcoderd/commit/0cbf476a92e33a92f807092f3d4b032c6f1ea776))
* update pgstosrt to latest version ([f3447da](https://github.com/segator/transcoderd/commit/f3447dafc34e796b82dd53c6d809132a0d058041))


### ♻️ Code Refactoring

* simplier code and more readable ([f3447da](https://github.com/segator/transcoderd/commit/f3447dafc34e796b82dd53c6d809132a0d058041))

## [2.1.0](https://github.com/segator/transcoderd/compare/v2.0.1...v2.1.0) (2025-01-20)


### 🎉 Features

* PGS conversion shows progress in worker cli ([b890256](https://github.com/segator/transcoderd/commit/b89025601403630e5153fb7d55a3fbfe0af6dac9))


### 🐛 Bug Fixes

* some fixes after linter corrections ([b890256](https://github.com/segator/transcoderd/commit/b89025601403630e5153fb7d55a3fbfe0af6dac9))
* uncomment something commented by accident ([277fbce](https://github.com/segator/transcoderd/commit/277fbcee8afa5353561838034512fb2ee51fe74a))


### 📝 Documentation

* update readme latest changes ([b890256](https://github.com/segator/transcoderd/commit/b89025601403630e5153fb7d55a3fbfe0af6dac9))

## [2.0.1](https://github.com/segator/transcoderd/compare/v2.0.0...v2.0.1) (2025-01-20)


### 🐛 Bug Fixes

* bad error handling ([ac9938d](https://github.com/segator/transcoderd/commit/ac9938dce9ecbfad651a7a22d3ebc20066018baf))

## [2.0.0](https://github.com/segator/transcoderd/compare/v1.8.0...v2.0.0) (2025-01-20)


### ⚠ BREAKING CHANGES

* centralize cli configuration, breaking change for config files and env vars

### 🎉 Features

* support Czech tesseract language ([7081843](https://github.com/segator/transcoderd/commit/7081843a244a90ad9fec2c8d599fdc1fa8a5ad2c))
* support greek teseract language ([d352cfa](https://github.com/segator/transcoderd/commit/d352cfaa179b0bed32185ed73f070f2b924547d0))
* support iceland teseract language ([d352cfa](https://github.com/segator/transcoderd/commit/d352cfaa179b0bed32185ed73f070f2b924547d0))


### 🐛 Bug Fixes

* allow grace time for update check as Github bans too many requests, by default 15min ([550f3a9](https://github.com/segator/transcoderd/commit/550f3a9dc67ebe9b6f05cef4b009443216863139))
* better cleaner for subtitle names if pgs ([f0c28b7](https://github.com/segator/transcoderd/commit/f0c28b7146b01879e2e78b9f0e6dfaf9b0316768))
* better error logs on PGS errors ([f0c28b7](https://github.com/segator/transcoderd/commit/f0c28b7146b01879e2e78b9f0e6dfaf9b0316768))
* bump pgstosrt due this bugfix https://github.com/Tentacule/PgsToSrt/issues/51 ([a9c05a6](https://github.com/segator/transcoderd/commit/a9c05a6de4b726b63d62a4d8953bfb1ceb2fcc80))
* change encode progress bar to support duration and frames as fallback for those cases ffmpeg can not calculate the timestamps ([550f3a9](https://github.com/segator/transcoderd/commit/550f3a9dc67ebe9b6f05cef4b009443216863139))
* error parsing time.duration parameters ([16056da](https://github.com/segator/transcoderd/commit/16056daaa7338c96aee7c9679b29cd418625b07b))
* if a PGS fails, make fail all job ([66b6002](https://github.com/segator/transcoderd/commit/66b600251f946da79ac7f48a2701046286ad5e8a))
* increased process buffer for performance ([550f3a9](https://github.com/segator/transcoderd/commit/550f3a9dc67ebe9b6f05cef4b009443216863139))
* PGS tasks now output stderr for extra debug info ([66b6002](https://github.com/segator/transcoderd/commit/66b600251f946da79ac7f48a2701046286ad5e8a))
* Upgrade db logs version to version was not correctly showing the current version if more than 1 db scheme upgrade was needed ([66b6002](https://github.com/segator/transcoderd/commit/66b600251f946da79ac7f48a2701046286ad5e8a))
* Wait for stdout/err command hook executed before leaving command exec ([66b6002](https://github.com/segator/transcoderd/commit/66b600251f946da79ac7f48a2701046286ad5e8a))


### 🤖 Continuous Integration

* enable linter ([52c39b2](https://github.com/segator/transcoderd/commit/52c39b27698b1c5436835231bda5da20b303a2a9))


### 🔧 Miscellaneous Chores

* format code ([c547847](https://github.com/segator/transcoderd/commit/c54784739b7bab03bd29bed1aad0031c4999e6f2))
* lint fixes ([7912861](https://github.com/segator/transcoderd/commit/791286155400fa8fd41fb49d8e1ad978f8e6c0f7))
* lint fixes ([f89adf5](https://github.com/segator/transcoderd/commit/f89adf554b1c1a084fd5195c41f94b12db41cf31))
* lint fixes ([f22ce8d](https://github.com/segator/transcoderd/commit/f22ce8d71429bd09078a994ceb74b3a0b9db2aa4))
* lint fixes ([81a875f](https://github.com/segator/transcoderd/commit/81a875fc70ba82dcfb26c7e0bc78e04daf7b2595))
* lint fixes ([09495ba](https://github.com/segator/transcoderd/commit/09495ba2d1f47d580ea235424c4e16206ca717b6))


### ♻️ Code Refactoring

* centralize cli configuration, breaking change for config files and env vars ([16056da](https://github.com/segator/transcoderd/commit/16056daaa7338c96aee7c9679b29cd418625b07b))

## [1.8.0](https://github.com/segator/transcoderd/compare/v1.7.0...v1.8.0) (2025-01-17)


### 🎉 Features

* server supports to cancel jobs ([c4618f8](https://github.com/segator/transcoderd/commit/c4618f81dc6ee93cd3260d68d9609fd4d76dbc18))


### 🔧 Miscellaneous Chores

* **deps:** update ffmpeg build script to latest ([c4618f8](https://github.com/segator/transcoderd/commit/c4618f81dc6ee93cd3260d68d9609fd4d76dbc18))
* **deps:** update how ffmpeg build script runs as seems not works for latest versions. ([84842f4](https://github.com/segator/transcoderd/commit/84842f4a2386b80bfe1d398a11daec3d55c76c12))

## [1.7.0](https://github.com/segator/transcoderd/compare/v1.6.1...v1.7.0) (2025-01-16)


### 🎉 Features

* server supports for auto update ([0678e12](https://github.com/segator/transcoderd/commit/0678e12442c6b47945c16e169a17b866e338c7f0))


### 🐛 Bug Fixes

* server was trying to retry some not retriable errors ([e259d1d](https://github.com/segator/transcoderd/commit/e259d1d8cceded72bd0240a350bed97ddb0fa6c2))


### 🤖 Continuous Integration

* allow to ignore cache by workflow dispatch ([4fe0b75](https://github.com/segator/transcoderd/commit/4fe0b75a2914efa9fad580e62c69ef101e4bfebf))
* broken ci ([8f05e18](https://github.com/segator/transcoderd/commit/8f05e18570d8af1f9d3891a58e2dec12331aebae))
* broken ci ([18ad558](https://github.com/segator/transcoderd/commit/18ad55874af86f378a9bed1aa1d0e236bb3dd089))
* docker cache in a separate image ([07bfbe6](https://github.com/segator/transcoderd/commit/07bfbe6e6acf15a579dd021614f3dfe6cca4124b))
* fix broken ci ([01a042d](https://github.com/segator/transcoderd/commit/01a042dd22b74b69dc64ab5883ec1b555f31be1a))
* invert cache bool var ([b2efcb0](https://github.com/segator/transcoderd/commit/b2efcb0154de1010a41b8f94c353f17f9e65ae60))

## [1.6.1](https://github.com/segator/transcoderd/compare/v1.6.0...v1.6.1) (2025-01-15)


### 🐛 Bug Fixes

* improve error handling on update.go ([d7649af](https://github.com/segator/transcoderd/commit/d7649af6ef9ba61e55f06ff6ac020f8d4c1d5701))
* improve error handling on update.go ([0d2a3ac](https://github.com/segator/transcoderd/commit/0d2a3ac271e236721658445d35febeca6d1f2c58))
* improve error handling on update.go ([f8e62cb](https://github.com/segator/transcoderd/commit/f8e62cb21ae336133b61630d973482662018f520))
* release-please not working properly? ([259e060](https://github.com/segator/transcoderd/commit/259e0609386c2f6036a49c51f8afe7325d7b0770))

## [1.6.0](https://github.com/segator/transcoderd/compare/v1.5.1...v1.6.0) (2025-01-15)


### Features

* cli flag to disable app auto updates ([78ad193](https://github.com/segator/transcoderd/commit/78ad19373c0ca31a8a1b14ba308929b0c9910a90))


### Bug Fixes

* tessdata support det lang ([6492f03](https://github.com/segator/transcoderd/commit/6492f03f0a1fb638fd2c25f8978a18608f86b444))

## [1.5.1](https://github.com/segator/transcoderd/compare/v1.5.0...v1.5.1) (2025-01-13)


### Bug Fixes

* updater were not comparing properly equal versions ([#11](https://github.com/segator/transcoderd/issues/11)) ([f566e21](https://github.com/segator/transcoderd/commit/f566e213b4db1e0f8b242ddf0234808d2699f124))

## [1.5.0](https://github.com/segator/transcoderd/compare/v1.4.0...v1.5.0) (2025-01-13)


### Features

* Enable back auto-updater ([#9](https://github.com/segator/transcoderd/issues/9)) ([7ad9859](https://github.com/segator/transcoderd/commit/7ad9859fa0e8f0a75cf16ea75a3b6b3ea00e1c92))

## [1.4.0](https://github.com/segator/transcoderd/compare/v1.3.0...v1.4.0) (2025-01-12)


### Features

* set proper versioning ([8c80754](https://github.com/segator/transcoderd/commit/8c80754ad10d6b6dbe658122b4fbea75699f5376))

## [1.3.0](https://github.com/segator/transcoderd/compare/v1.2.0...v1.3.0) (2025-01-12)


### Features

* set proper versioning ([8f83249](https://github.com/segator/transcoderd/commit/8f832494fc1a014027acbf378e7c2587583e0377))

## [1.2.0](https://github.com/segator/transcoderd/compare/v1.1.0...v1.2.0) (2025-01-12)


### Features

* now is gonna work ([2531c06](https://github.com/segator/transcoderd/commit/2531c067da9cfd17815d1c00ef5bd9d2e77780d3))

## [1.1.0](https://github.com/segator/transcoderd/compare/v1.0.0...v1.1.0) (2025-01-12)


### Features

* release-please more tests ([8f5e1ae](https://github.com/segator/transcoderd/commit/8f5e1ae14bcbea34d49b548b553b02efc177ce19))

## 1.0.0 (2025-01-12)


### Features

* testing release-please ([a0c111d](https://github.com/segator/transcoderd/commit/a0c111d5ed8649d8de4d6befece4b3a756af0703))
* testing release-please ([16d92dc](https://github.com/segator/transcoderd/commit/16d92dc34fc8aa5b8d101e8413f0e5969af88ab1))
* testing release-please ([cd5122e](https://github.com/segator/transcoderd/commit/cd5122e0d3ad28c16b4c227ac7ce8f1e28835e70))

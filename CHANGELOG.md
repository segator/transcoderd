# Changelog

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

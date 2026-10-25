# Tasks

- [x] 1. Add best-effort cleanup for removed destination-local and legacy staging files, deleting only empty `.transcoderd-upload` directories.
- [x] 2. Make upload-attempt creation retry if cleanup removes the staging directory between directory creation and file opening.
- [x] 3. Add regression tests for destination-local cleanup, legacy cleanup, and preservation of a non-empty directory with another upload file.
- [x] 4. Run focused scheduler tests and strict OpenSpec validation.

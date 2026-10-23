# Spec Delta

## MODIFIED Requirements

### Requirement: Destination naming
The scheduler SHALL generate a provisional `.mkv` upload destination path, replacing recognized source video codec labels with `x265` and recognized source audio codec labels with `AAC`. If that path equals the source path, it SHALL add `_encoded.mkv`. The server SHALL stage each upload in a job-specific path separate from final destinations. Staging and finalization SHALL work when the staging root and media destination are on different filesystems. After upload, the server SHALL probe the output video stream, finalize a non-conflicting path that reflects the actual codec, and persist that path before recording the job as completed.

#### Scenario: Codec labels in the name
- **WHEN** the scheduler accepts the source path `shows/episode.x264.dts.avi`
- **THEN** the provisional upload destination path is `shows/episode.x265.AAC.mkv`

#### Scenario: H.265 and HEVC source labels are recognized
- **WHEN** the source path contains an `h265` or `hevc` label and the uploaded output's video codec is `av1`
- **THEN** the generated destination path replaces that label with `av1`

#### Scenario: Name otherwise unchanged
- **WHEN** the source path is `movies/film.mkv` and no video or audio codec label changes
- **THEN** the provisional upload destination path is `movies/film_encoded.mkv`

#### Scenario: H.264 source encoded to AV1
- **WHEN** the source path is `movies/film.x264.AC3.mkv` and the uploaded output's video codec is `av1`
- **THEN** the final destination path is `movies/film.av1.AAC.mkv`

#### Scenario: HEVC source encoded to AV1
- **WHEN** the source path is `movies/film.x265.AAC.mkv` and the uploaded output's video codec is `av1`
- **THEN** the final destination path is `movies/film.av1.AAC.mkv`

#### Scenario: AV1 source encoded to HEVC
- **WHEN** the source path is `movies/film.av1.AAC.mkv` and the uploaded output's video codec is `hevc`
- **THEN** the final destination path is `movies/film.x265.AAC.mkv`

#### Scenario: Output filename has no codec label
- **WHEN** the source path is `movies/film.mkv` and the uploaded output's video codec is `av1`
- **THEN** the final destination path is `movies/film.av1.mkv`

#### Scenario: Final path would overwrite an existing file
- **WHEN** the codec-derived final destination already exists and is not the source file
- **THEN** the server preserves the existing file and chooses the next available alternative name, such as `_encoded.mkv`, `_encoded-2.mkv`, or a higher numbered suffix, until it finds a free destination

#### Scenario: Output codec cannot be verified
- **WHEN** the server cannot probe the uploaded output or it contains no video stream
- **THEN** the job is not recorded as completed and the source file is not deleted

#### Scenario: Staging and library are on different filesystems
- **WHEN** an upload is staged on a filesystem different from the final media destination
- **THEN** the server transfers it to a destination-local staging file and finalizes it without overwriting an existing file
- **AND** the server records the final path and size before recording the job as completed

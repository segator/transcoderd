package step

import (
	"context"
	"crypto/sha256"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"regexp"
	"strconv"
	"strings"
	"time"

	"transcoder/helper"
	"transcoder/helper/command"
	"transcoder/model"
	"transcoder/worker/config"
	"transcoder/worker/console"
	"transcoder/worker/ffmpeg"
	"transcoder/worker/job"

	"gopkg.in/ini.v1"
)

type FFMPEGStepExecutor struct {
	ffmpegConfig *config.FFMPEGConfig
}

func NewFFMPEGStepExecutor(ffmpegConfig *config.FFMPEGConfig, options ...ExecutorOption) *Executor {
	ffmpegStep := &FFMPEGStepExecutor{
		ffmpegConfig,
	}
	return NewStepExecutor(model.FFMPEGSNotification, ffmpegStep.actions, options...)
}

func (f *FFMPEGStepExecutor) actions(jobContext *job.Context) []Action {
	return []Action{
		{
			Execute: func(ctx context.Context, stepTracker Tracker) error {
				return f.encode(ctx, stepTracker, jobContext)
			},
			Id: jobContext.JobId.String(),
		},
	}

}

func (f *FFMPEGStepExecutor) encode(ctx context.Context, stepTracker Tracker, jobContext *job.Context) error {
	FFMPEGProgressChan := make(chan int64, 1)
	go f.ffmpegProgressRoutine(ctx, jobContext, stepTracker, FFMPEGProgressChan)
	err := f.ffmpeg(ctx, stepTracker.Logger(), jobContext, FFMPEGProgressChan)
	if err != nil {
		return err
	}

	return nil
}

func (f *FFMPEGStepExecutor) ffmpegProgressRoutine(ctx context.Context, job *job.Context, tracker Tracker, ffmpegProgressChan chan int64) {
	tracker.SetTotal(int64(job.Source.FFProbeData.Video.Duration.Seconds()) * int64(job.Source.FFProbeData.Video.FrameRate))
	for {
		select {
		case <-ctx.Done():
			return
		case progress, open := <-ffmpegProgressChan:
			if !open {
				return
			}
			tracker.UpdateValue(progress)
		}
	}
}

func (f *FFMPEGStepExecutor) ffmpeg(ctx context.Context, logger console.LeveledLogger, jobContext *job.Context, ffmpegProgressChan chan<- int64) error {
	defer close(ffmpegProgressChan)
	ffmpegGenerator := &FFMPEGGenerator{Config: f.ffmpegConfig}
	ffmpegGenerator.setInputFilters(jobContext)
	if err := ffmpegGenerator.setVideoFilters(jobContext.Source.FFProbeData); err != nil {
		return err
	}
	ffmpegGenerator.setAudioFilters(jobContext.Source.FFProbeData)
	ffmpegGenerator.setSubtFilters(jobContext.Source.FFProbeData)
	ffmpegErrLog := ""

	checkPercentageFFMPEG := func(buffer []byte, exit bool) {
		ffmpegErrLog += string(buffer)
	}

	stdoutFFMPEG := func(buffer []byte, exit bool) {
		cfg, err := ini.Load(buffer)
		if err != nil {
			return
		}
		s := cfg.Section("")
		progress := s.Key("progress").String()
		if progress == "continue" {
			var progressValue int64
			outTimeUs, err := s.Key("out_time_ms").Int64()
			if err == nil {
				progressValue = (outTimeUs / 1000000) * int64(jobContext.Source.FFProbeData.Video.FrameRate)
			}
			// If out_time_ms is not present, we can use frame as a fallback, even is not as precise
			if progressValue == 0 {
				frame, err := s.Key("frame").Int64()
				if err != nil {
					return
				}
				progressValue = frame
			}

			select {
			case ffmpegProgressChan <- progressValue:
			default:
			}

		}
	}
	sourceFileName := filepath.Base(jobContext.Source.FilePath)
	encodedFilePath := fmt.Sprintf("%s-encoded.%s", strings.TrimSuffix(sourceFileName, filepath.Ext(sourceFileName)), "mkv")
	targetPath := filepath.Join(jobContext.WorkingDir, encodedFilePath)

	ffmpegArguments := ffmpegGenerator.buildArguments(uint8(f.ffmpegConfig.Threads), f.ffmpegConfig.ExtraArgs, targetPath)
	ffmpegCommand := command.NewCommand(helper.GetFFmpegPath(), ffmpegArguments...)
	logger.Cmdf("FFMPEG Command: %s", ffmpegCommand.GetFullCommand())
	ffmpegCommand = ffmpegCommand.
		SetWorkDir(jobContext.WorkingDir).
		SetStdoutFunc(stdoutFFMPEG).
		SetStderrFunc(checkPercentageFFMPEG)

	exitCode, err := ffmpegCommand.RunWithContext(ctx)
	if err != nil {
		return fmt.Errorf("%w: stder:%s", err, ffmpegErrLog)
	}
	if exitCode != 0 {
		return fmt.Errorf("exit code %d: stder:%s", exitCode, ffmpegErrLog)
	}

	<-time.After(time.Second * 1)
	ffprobeData, err := ffmpeg.ExtractFFProbeData(ctx, targetPath)
	if err != nil {
		return err
	}

	normalizedFFProbeData, err := ffmpeg.NormalizeFFProbeData(ffprobeData)
	if err != nil {
		return err
	}

	jobContext.TargetMediaProbe = ffmpeg.ProbeDataToMediaProbe(ffprobeData)

	sha256str, err := hashFileSHA256(targetPath)
	if err != nil {
		return err
	}
	targetInfo, err := os.Stat(targetPath)
	if err != nil {
		return fmt.Errorf("stat encoded file: %w", err)
	}

	jobContext.Target = &job.VideoData{
		FilePath:    targetPath,
		Checksum:    sha256str,
		FFProbeData: normalizedFFProbeData,
		Size:        targetInfo.Size(),
	}
	return nil
}

type FFMPEGGenerator struct {
	Config         *config.FFMPEGConfig
	inputPaths     []string
	VideoFilter    []string
	AudioFilter    [][]string
	SubtitleFilter [][]string
}

func (f *FFMPEGGenerator) setAudioFilters(container *ffmpeg.NormalizedFFProbe) {
	for index, audioStream := range container.Audios {
		title := fmt.Sprintf("%s (%s)", audioStream.Language, audioStream.ChannelLayour)
		f.AudioFilter = append(f.AudioFilter, []string{
			"-map", fmt.Sprintf("0:%d", audioStream.Id),
			fmt.Sprintf("-metadata:s:a:%d", index), "title=" + title,
			fmt.Sprintf("-c:a:%d", index), f.Config.AudioCodec,
			"-vbr", strconv.Itoa(f.Config.AudioVBR),
		})
	}
}
func (f *FFMPEGGenerator) setVideoFilters(container *ffmpeg.NormalizedFFProbe) error {
	settings, err := f.Config.ResolveVideoSettings()
	if err != nil {
		return err
	}
	f.VideoFilter = []string{
		"-map", fmt.Sprintf("0:%d", container.Video.Id),
		"-avoid_negative_ts", "make_zero", "-copyts", "-map_chapters", "-1",
		"-flags", "+global_header",
		"-filter:v", "scale='min(1920,iw)':-1:force_original_aspect_ratio=decrease",
		"-pix_fmt", settings.PixelFormat, "-c:v", settings.Codec,
	}
	switch settings.Codec {
	case config.AV1NVENC:
		f.VideoFilter = append(f.VideoFilter,
			"-preset", settings.Preset, "-tune", "uhq", "-rc", "vbr",
			"-cq", strconv.Itoa(settings.CQ), "-b:v", "0", "-multipass", "fullres",
			"-bf", "4", "-b_ref_mode", "middle", "-split_encode_mode", "disabled",
			"-qmin", "0", "-qmax", strconv.Itoa(settings.QMax))
	case config.AV1SVT:
		f.VideoFilter = append(f.VideoFilter, "-preset", settings.Preset, "-crf", strconv.Itoa(settings.CRF))
	case config.HEVCX265:
		f.VideoFilter = append(f.VideoFilter, "-crf", strconv.Itoa(settings.CRF),
			"-profile:v", settings.Profile, "-preset", settings.Preset)
	}
	return nil
}
func (f *FFMPEGGenerator) setSubtFilters(container *ffmpeg.NormalizedFFProbe) {
	subtInputIndex := 1
	outputIndex := 0
	for _, subtitle := range container.Subtitle {
		if subtitle.IsImageTypeSubtitle() || subtitle.NeedsMKVExtraction() {
			// PGS (OCR'd to SRT) and externally extracted unsupported codecs are
			// fed as separate input files. Map from the corresponding input index.
			subtitleArgs := []string{"-map", fmt.Sprintf("%d:0", subtInputIndex),
				fmt.Sprintf("-c:s:%d", outputIndex), "srt"}

			// Build disposition flags: combine into a single -disposition flag
			// to avoid FFmpeg overwriting earlier values and to use correct stream specifier (s:N not s:s:N)
			var dispositions []string
			if subtitle.Forced {
				dispositions = append(dispositions, "forced", "default")
			}
			if subtitle.Comment {
				dispositions = append(dispositions, "comment")
			}
			if len(dispositions) > 0 {
				subtitleArgs = append(subtitleArgs, fmt.Sprintf("-disposition:s:%d", outputIndex), strings.Join(dispositions, "+"))
			}

			// Clean subtitle title to avoid PGS in title
			re := regexp.MustCompile(`(?i)\(?pgs\)?`)
			subtitleTitle := re.ReplaceAllString(subtitle.Title, "")
			subtitleTitle = strings.TrimSpace(strings.ReplaceAll(subtitleTitle, "  ", " "))

			subtitleArgs = append(subtitleArgs,
				fmt.Sprintf("-metadata:s:s:%d", outputIndex), "language="+subtitle.Language,
				fmt.Sprintf("-metadata:s:s:%d", outputIndex), "title="+subtitleTitle,
				"-max_interleave_delta", "0")
			f.SubtitleFilter = append(f.SubtitleFilter, subtitleArgs)
			subtInputIndex++
			outputIndex++
		} else if subtitle.NeedsDirectSRTConversion() {
			subtitleArgs := []string{"-map", fmt.Sprintf("0:%d", subtitle.Id),
				fmt.Sprintf("-c:s:%d", outputIndex), "srt"}
			var dispositions []string
			if subtitle.Forced {
				dispositions = append(dispositions, "forced", "default")
			}
			if subtitle.Comment {
				dispositions = append(dispositions, "comment")
			}
			if len(dispositions) > 0 {
				subtitleArgs = append(subtitleArgs, fmt.Sprintf("-disposition:s:%d", outputIndex), strings.Join(dispositions, "+"))
			}
			subtitleArgs = append(subtitleArgs,
				fmt.Sprintf("-metadata:s:s:%d", outputIndex), "language="+subtitle.Language,
				fmt.Sprintf("-metadata:s:s:%d", outputIndex), "title="+subtitle.Title,
				"-max_interleave_delta", "0")
			f.SubtitleFilter = append(f.SubtitleFilter, subtitleArgs)
			outputIndex++
		} else {
			f.SubtitleFilter = append(f.SubtitleFilter, []string{"-map", fmt.Sprintf("0:%d", subtitle.Id),
				fmt.Sprintf("-c:s:%d", outputIndex), "copy"})
			outputIndex++
		}
	}
}

func (f *FFMPEGGenerator) buildArguments(threads uint8, extraArgs string, outputFilePath string) []string {
	args := []string{"-fflags", "+genpts", "-nostats"}
	if extraArgs != "" {
		args = append(args, command.StringToSlice(extraArgs)...)
	}
	args = append(args, "-progress", "pipe:1", "-hide_banner", "-nostdin",
		"-threads", strconv.Itoa(int(threads)), "-analyzeduration", "2147483647",
		"-probesize", "2147483647")
	for _, input := range f.inputPaths {
		args = append(args, "-i", input)
	}
	args = append(args, "-max_muxing_queue_size", "9999")
	args = append(args, f.VideoFilter...)
	for _, audio := range f.AudioFilter {
		args = append(args, audio...)
	}
	for _, subtitle := range f.SubtitleFilter {
		args = append(args, subtitle...)
	}
	return append(args, "-y", outputFilePath)
}

func (f *FFMPEGGenerator) setInputFilters(jobContext *job.Context) {
	source := jobContext.Source
	f.inputPaths = append(f.inputPaths, source.FilePath)
	// Add PGS and extracted subtitle SRTs in source track order so their FFmpeg
	// input indexes match the order used by setSubtFilters.
	for _, subt := range source.FFProbeData.Subtitle {
		if subt.IsImageTypeSubtitle() || subt.NeedsMKVExtraction() {
			srtPath := filepath.Join(jobContext.WorkingDir, fmt.Sprintf("%d.srt", subt.Id))
			f.inputPaths = append(f.inputPaths, srtPath)
		}
	}
}

func hashFileSHA256(filePath string) (string, error) {
	file, err := os.Open(filePath)
	if err != nil {
		return "", err
	}
	defer file.Close()

	hash := sha256.New()
	if _, err := io.Copy(hash, file); err != nil {
		return "", err
	}

	return fmt.Sprintf("%x", hash.Sum(nil)), nil
}

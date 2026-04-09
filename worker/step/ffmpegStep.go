package step

import (
	"context"
	"crypto/sha256"
	"fmt"
	"gopkg.in/ini.v1"
	"io"
	"os"
	"path/filepath"
	"regexp"
	"runtime"
	"strings"
	"time"
	"transcoder/helper"
	"transcoder/helper/command"
	"transcoder/model"
	"transcoder/worker/config"
	"transcoder/worker/console"
	"transcoder/worker/ffmpeg"
	"transcoder/worker/job"
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
	FFMPEGProgressChan := make(chan int64)
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
	ffmpegGenerator := &FFMPEGGenerator{Config: f.ffmpegConfig}
	ffmpegGenerator.setInputFilters(jobContext)
	ffmpegGenerator.setVideoFilters(jobContext.Source.FFProbeData)
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

			ffmpegProgressChan <- progressValue

		}
		if exit {
			close(ffmpegProgressChan)
		}
	}
	sourceFileName := filepath.Base(jobContext.Source.FilePath)
	encodedFilePath := fmt.Sprintf("%s-encoded.%s", strings.TrimSuffix(sourceFileName, filepath.Ext(sourceFileName)), "mkv")
	targetPath := filepath.Join(jobContext.WorkingDir, encodedFilePath)

	ffmpegArguments := ffmpegGenerator.buildArguments(uint8(f.ffmpegConfig.Threads), f.ffmpegConfig.ExtraArgs, targetPath)
	logger.Cmdf("FFMPEG Command:%s %s", helper.GetFFmpegPath(), ffmpegArguments)
	ffmpegCommand := command.NewCommandByString(helper.GetFFmpegPath(), ffmpegArguments).
		SetWorkDir(jobContext.WorkingDir).
		SetStdoutFunc(stdoutFFMPEG).
		SetStderrFunc(checkPercentageFFMPEG)

	if runtime.GOOS == "linux" {
		ffmpegCommand.AddEnv(fmt.Sprintf("LD_LIBRARY_PATH=%s", filepath.Dir(helper.GetFFmpegPath())))
	}
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

	sha256str, err := hashFileSHA256(targetPath)
	if err != nil {
		return err
	}

	jobContext.Target = &job.VideoData{
		FilePath:    targetPath,
		Checksum:    sha256str,
		FFProbeData: normalizedFFProbeData,
	}
	return nil
}

type FFMPEGGenerator struct {
	Config         *config.FFMPEGConfig
	inputPaths     []string
	VideoFilter    string
	AudioFilter    []string
	SubtitleFilter []string
	Metadata       string
}

func (f *FFMPEGGenerator) setAudioFilters(container *ffmpeg.NormalizedFFProbe) {

	for index, audioStream := range container.Audios {
		// TODO que pasa quan el channelLayout esta empty??
		title := fmt.Sprintf("%s (%s)", audioStream.Language, audioStream.ChannelLayour)
		metadata := fmt.Sprintf(" -metadata:s:a:%d \"title=%s\"", index, title)
		codecQuality := fmt.Sprintf("-c:a:%d %s -vbr %d", index, f.Config.AudioCodec, f.Config.AudioVBR)
		f.AudioFilter = append(f.AudioFilter, fmt.Sprintf(" -map 0:%d %s %s", audioStream.Id, metadata, codecQuality))
	}
}
func (f *FFMPEGGenerator) setVideoFilters(container *ffmpeg.NormalizedFFProbe) {
	videoFilterParameters := "\"scale='min(1920,iw)':-1:force_original_aspect_ratio=decrease\""
	videoEncoderQuality := fmt.Sprintf("-pix_fmt yuv420p10le -c:v %s -crf %d -profile:v %s -preset %s", f.Config.VideoCodec, f.Config.VideoCRF, f.Config.VideoProfile, f.Config.VideoPreset)
	// TODO HDR??
	videoHDR := ""
	f.VideoFilter = fmt.Sprintf("-map 0:%d -avoid_negative_ts make_zero -copyts -map_chapters -1 -flags +global_header -filter:v %s %s %s", container.Video.Id, videoFilterParameters, videoHDR, videoEncoderQuality)

}
func (f *FFMPEGGenerator) setSubtFilters(container *ffmpeg.NormalizedFFProbe) {
	subtInputIndex := 1
	outputIndex := 0
	for _, subtitle := range container.Subtitle {
		if subtitle.IsImageTypeSubtitle() || subtitle.NeedsMKVExtraction() {
			// Both PGS (OCR'd to SRT) and extracted unsupported codecs (converted to SRT)
			// are fed as separate input files. Map from the corresponding input index.
			subtitleMap := fmt.Sprintf("-map %d -c:s:%d srt", subtInputIndex, outputIndex)
			subtitleForced := ""
			subtitleComment := ""
			if subtitle.Forced {
				subtitleForced = fmt.Sprintf(" -disposition:s:s:%d forced  -disposition:s:s:%d default", outputIndex, outputIndex)
			}
			if subtitle.Comment {
				subtitleComment = fmt.Sprintf(" -disposition:s:s:%d comment", outputIndex)
			}

			// Clean subtitle title to avoid PGS in title
			re := regexp.MustCompile(`(?i)\(?pgs\)?`)
			subtitleTitle := re.ReplaceAllString(subtitle.Title, "")
			subtitleTitle = strings.TrimSpace(strings.ReplaceAll(subtitleTitle, "  ", " "))

			f.SubtitleFilter = append(f.SubtitleFilter, fmt.Sprintf("%s %s %s -metadata:s:s:%d language=%s -metadata:s:s:%d \"title=%s\" -max_interleave_delta 0", subtitleMap, subtitleForced, subtitleComment, outputIndex, subtitle.Language, outputIndex, subtitleTitle))
			subtInputIndex++
			outputIndex++
		} else {
			f.SubtitleFilter = append(f.SubtitleFilter, fmt.Sprintf("-map 0:%d -c:s:%d copy", subtitle.Id, outputIndex))
			outputIndex++
		}
	}
}

func (f *FFMPEGGenerator) buildArguments(threads uint8, extraArgs string, outputFilePath string) string {
	coreParameters := fmt.Sprintf("-fflags +genpts -nostats %s -progress pipe:1  -hide_banner  -threads %d -analyzeduration 2147483647 -probesize 2147483647", extraArgs, threads)
	inputsParameters := ""
	for _, input := range f.inputPaths {
		inputsParameters = fmt.Sprintf("%s -i \"%s\"", inputsParameters, input)
	}
	//-ss 900 -t 10
	audioParameters := ""
	for _, audio := range f.AudioFilter {
		audioParameters = fmt.Sprintf("%s %s", audioParameters, audio)
	}
	subtParameters := ""
	for _, subt := range f.SubtitleFilter {
		subtParameters = fmt.Sprintf("%s %s", subtParameters, subt)
	}

	return fmt.Sprintf("%s %s -max_muxing_queue_size 9999 %s %s %s %s %s -y", coreParameters, inputsParameters, f.VideoFilter, audioParameters, subtParameters, f.Metadata, outputFilePath)
}

func (f *FFMPEGGenerator) setInputFilters(jobContext *job.Context) {
	source := jobContext.Source
	f.inputPaths = append(f.inputPaths, source.FilePath)
	// Add PGS-to-SRT converted files as separate inputs
	if source.FFProbeData.HaveImageTypeSubtitle() {
		for _, subt := range source.FFProbeData.Subtitle {
			if subt.IsImageTypeSubtitle() {
				srtEncodedFile := filepath.Join(jobContext.WorkingDir, fmt.Sprintf("%d.srt", subt.Id))
				f.inputPaths = append(f.inputPaths, srtEncodedFile)
			}
		}
	}
	// Add extracted-and-converted subtitle files (e.g. WebVTT -> SRT) as separate inputs
	for _, subt := range source.FFProbeData.GetExtractableSubtitles() {
		srtConvertedFile := filepath.Join(jobContext.WorkingDir, fmt.Sprintf("%d.srt", subt.Id))
		f.inputPaths = append(f.inputPaths, srtConvertedFile)
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

package step

import (
	"context"
	"errors"
	"fmt"
	"github.com/asticode/go-astisub"
	"os"
	"regexp"
	"strconv"
	"strings"
	"time"
	"transcoder/helper/command"
	"transcoder/model"
	"transcoder/worker/config"
	"transcoder/worker/ffmpeg"
	"transcoder/worker/job"
)

type PGSToSrtStepExecutor struct {
	pgsConfig *config.PGSConfig
}

func NewPGSToSrtStepExecutor(pgsConfig *config.PGSConfig, opts ...ExecutorOption) *Executor {
	pgsStep := &PGSToSrtStepExecutor{
		pgsConfig: pgsConfig,
	}

	return NewStepExecutor(model.PGSNotification, pgsStep.actions, opts...)
}

func (p *PGSToSrtStepExecutor) actions(jobContext *job.Context) []Action {
	var pgsStepActions []Action
	for _, pgs := range jobContext.Source.FFProbeData.GetPGSSubtitles() {
		pgsStepActions = append(pgsStepActions, Action{
			Execute: func(ctx context.Context, stepTracker Tracker) error {
				return p.convertPGSToSrt(ctx, stepTracker, jobContext, pgs)
			},
			Id: fmt.Sprintf("%s %d", jobContext.JobId, pgs.Id),
		})
	}
	return pgsStepActions
}

func (p *PGSToSrtStepExecutor) convertPGSToSrt(ctx context.Context, tracker Tracker, jobContext *job.Context, subtitle *ffmpeg.Subtitle) error {
	pgsConfig := p.pgsConfig
	inputFilePath := fmt.Sprintf("%s/%d.sup", jobContext.WorkingDir, subtitle.Id)
	outputFilePath := fmt.Sprintf("%s/%d.srt", jobContext.WorkingDir, subtitle.Id)
	language := calculateTesseractLanguage(subtitle.Language)

	PGSToSrtCommand := command.NewCommand(pgsConfig.DotnetPath, pgsConfig.DLLPath,
		"--tesseractversion", strconv.Itoa(pgsConfig.TessVersion),
		"--libleptname", pgsConfig.LibleptName,
		"--libleptversion", strconv.Itoa(pgsConfig.LibleptVersion),
		"--input", inputFilePath,
		"--output", outputFilePath,
		"--tesseractlanguage", language,
		"--tesseractdata", pgsConfig.TesseractDataPath).SetWorkDir(jobContext.WorkingDir)
	outLog := ""
	startRegex := regexp.MustCompile(`Starting OCR for (\d+) items`)
	progressRegex := regexp.MustCompile(`Processed item (\d+)`)
	PGSToSrtCommand.SetStdoutFunc(func(buffer []byte, exit bool) {
		str := string(buffer)
		outLog += str
		progressMatch := progressRegex.FindStringSubmatch(str)
		if len(progressMatch) > 0 {
			p, err := strconv.Atoi(progressMatch[len(progressMatch)-1])
			if err != nil {
				return
			}
			tracker.UpdateValue(int64(p))
		}
		startMatch := startRegex.FindStringSubmatch(str)
		if len(startMatch) > 0 {
			t, err := strconv.Atoi(startMatch[1])
			if err != nil {
				return
			}
			tracker.SetTotal(int64(t))
		}

	})
	errLog := ""
	PGSToSrtCommand.SetStderrFunc(func(buffer []byte, exit bool) {
		errLog += string(buffer)
	})
	tracker.Logger().Cmdf("PGSTOSrt Command: %s", PGSToSrtCommand.GetFullCommand())
	ecode, err := PGSToSrtCommand.RunWithContext(ctx)
	pgslog := fmt.Sprintf("stdout: %s, stderr: %s", outLog, errLog)
	if err != nil {
		return fmt.Errorf("%v: %s", err, pgslog)
	}
	if ecode != 0 {
		return fmt.Errorf("invalid exit code %d: %s", ecode, pgslog)
	}
	langNotFound := fmt.Sprintf("Language '%s' is not available in Tesseract data directory", language)
	if strings.Contains(outLog, langNotFound) {
		return errors.New(langNotFound)
	}

	if !strings.Contains(outLog, "Finished OCR.") {
		return fmt.Errorf("PGSToSrt no Finished OCR line: %s", pgslog)
	}

	if strings.Contains(outLog, "with 0 items.") {
		return fmt.Errorf("no items converted: %s", pgslog)
	}

	subtitles, err := astisub.OpenFile(outputFilePath)
	if err != nil {
		return fmt.Errorf("could not parse subtitles: %v: %s", err, pgslog)
	}

	for _, item := range subtitles.Items {
		// This is an special case, pgstosrt some times set end time to 00:00 or lower than start time
		if item.StartAt > item.EndAt {
			item.EndAt = item.StartAt + (2 * time.Second)
		}
	}
	subtitles.Optimize()
	subtitles.Unfragment()
	outFile, err := os.OpenFile(outputFilePath, os.O_WRONLY|os.O_TRUNC, 0644)
	if err != nil {
		return fmt.Errorf("could not open file for writing: %v", err)
	}
	defer outFile.Close()
	if err = subtitles.WriteToSRT(outFile); err != nil {
		return fmt.Errorf("could not write to file: %v", err)
	}

	return err
}

var langMapping []PGSTesseractLanguage

type PGSTesseractLanguage struct {
	tessLanguage    string
	mappingLanguage []string
}

func init() {
	langMapping = append(langMapping, PGSTesseractLanguage{"deu", []string{"ger", "ge", "de"}})
	langMapping = append(langMapping, PGSTesseractLanguage{"eus", []string{"baq", "eus"}})
	langMapping = append(langMapping, PGSTesseractLanguage{"eng", []string{"en", "uk"}})
	langMapping = append(langMapping, PGSTesseractLanguage{"spa", []string{"es", "esp"}})
	langMapping = append(langMapping, PGSTesseractLanguage{"deu", []string{"det"}})
	langMapping = append(langMapping, PGSTesseractLanguage{"fra", []string{"fre"}})
	langMapping = append(langMapping, PGSTesseractLanguage{"chi_tra", []string{"chi"}})
	langMapping = append(langMapping, PGSTesseractLanguage{"ell", []string{"gre"}})
	langMapping = append(langMapping, PGSTesseractLanguage{"isl", []string{"ice"}})
	langMapping = append(langMapping, PGSTesseractLanguage{"ces", []string{"cze"}})
	langMapping = append(langMapping, PGSTesseractLanguage{"ron", []string{"rum"}})
}

func calculateTesseractLanguage(language string) string {
	for _, mapping := range langMapping {
		for _, mapLang := range mapping.mappingLanguage {
			if language == mapLang {
				return mapping.tessLanguage
			}
		}
	}
	return language
}

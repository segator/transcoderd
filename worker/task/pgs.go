package task

import (
	"context"
	"errors"
	"fmt"
	"github.com/asticode/go-astisub"
	log "github.com/sirupsen/logrus"
	"os"
	"strconv"
	"strings"
	"time"
	"transcoder/helper/command"
	"transcoder/model"
)

var langMapping []PGSTesseractLanguage

type PGSWorker struct {
	workerConfig Config
}

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
}
func NewPGSWorker(workerConfig Config) *PGSWorker {
	encodeWorker := &PGSWorker{
		workerConfig: workerConfig,
	}
	return encodeWorker
}

func (P *PGSWorker) ConvertPGS(ctx context.Context, taskPGS model.TaskPGS) (err error) {
	log.Debugf("Converting PGS To Srt for Job stream %d", taskPGS.PGSID)
	inputFilePath := taskPGS.PGSSourcePath
	outputFilePath := taskPGS.PGSTargetPath

	language := calculateTesseractLanguage(taskPGS.PGSLanguage)
	pgsConfig := P.workerConfig.PGSConfig

	PGSToSrtCommand := command.NewCommand(pgsConfig.DotnetPath, pgsConfig.DLLPath,
		"--tesseractversion", strconv.Itoa(pgsConfig.TessVersion),
		"--libleptname", pgsConfig.LibleptName,
		"--libleptversion", strconv.Itoa(pgsConfig.LibleptVersion),
		"--input", inputFilePath,
		"--output", outputFilePath,
		"--tesseractlanguage", language,
		"--tesseractdata", pgsConfig.TesseractDataPath)
	outLog := ""
	PGSToSrtCommand.SetStdoutFunc(func(buffer []byte, exit bool) {
		outLog += string(buffer)
	})
	errLog := ""
	PGSToSrtCommand.SetStderrFunc(func(buffer []byte, exit bool) {
		errLog += string(buffer)
	})
	log.Debugf("PGSTOSrt Command: %s", PGSToSrtCommand.GetFullCommand())
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

	log.Debugf("Converted PGS To Srt for Job stream %d", taskPGS.PGSID)
	return err
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

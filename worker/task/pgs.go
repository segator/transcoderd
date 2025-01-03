package task

import (
	"context"
	"errors"
	"fmt"
	log "github.com/sirupsen/logrus"
	"os"
	"strconv"
	"strings"
	"transcoder/helper/command"
	"transcoder/model"
)

var langMapping []PGSTesseractLanguage

type PGSWorker struct {
	workerConfig Config
	name         string
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
	langMapping = append(langMapping, PGSTesseractLanguage{"fra", []string{"fre"}})
	langMapping = append(langMapping, PGSTesseractLanguage{"chi_tra", []string{"chi"}})
}
func NewPGSWorker(workerConfig Config, workerName string) *PGSWorker {
	encodeWorker := &PGSWorker{
		name:         workerName,
		workerConfig: workerConfig,
	}
	return encodeWorker
}

func (P *PGSWorker) ConvertPGS(ctx context.Context, taskPGS model.TaskPGS) (err error) {
	log.Infof("Converting PGS To Srt for Job stream %d", taskPGS.PGSID)
	//TODO events??
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
	log.Debugf("PGSTOSrt Command: %s", PGSToSrtCommand.GetFullCommand())
	ecode, err := PGSToSrtCommand.RunWithContext(ctx)
	if err != nil {
		return err
	}
	if ecode != 0 {
		return errors.New(fmt.Sprintf("PGSToSrt invalid exit code %d", ecode))
	}
	langNotFound := fmt.Sprintf("Language '%s' is not available in Tesseract data directory", language)
	if strings.Contains(outLog, langNotFound) {
		return errors.New(langNotFound)
	}

	if !strings.Contains(outLog, "Finished OCR.") {
		return fmt.Errorf("PGSToSrt failed: %s", outLog)
	}

	if strings.Contains(outLog, "with 0 items.") {
		// This could happens if the PGS file is empty, in such case we delete the empty generated srt
		os.Remove(outputFilePath)
		return nil
	}

	f, err := os.Open(outputFilePath)
	if err != nil {
		return err
	}
	defer f.Close()
	log.Infof("Converted PGS To Srt for Job stream %d", taskPGS.PGSID)
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

func (P PGSWorker) GetID() string {
	return P.name
}

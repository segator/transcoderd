package console

import (
	"context"
	"github.com/jedib0t/go-pretty/v6/progress"
	"github.com/jedib0t/go-pretty/v6/text"
	log "github.com/sirupsen/logrus"
	"sync"
	"time"
	"transcoder/model"
)

var (
	unitScales = []int64{
		1000000000000000,
		1000000000000,
		1000000000,
		1000000,
		1000,
	}
)

type RenderService struct {
	pw progress.Writer
}

func NewRenderService() *RenderService {
	pw := newProgressWriter()
	return &RenderService{
		pw: pw,
	}
}
func (e *RenderService) Run(wg *sync.WaitGroup, ctx context.Context) {
	log.Info("Starting Console...")
	go e.pw.Render()
	wg.Add(1)
	go func() {
		<-ctx.Done()
		e.pw.Stop()
		log.Info("Stopping Console...")
		wg.Done()
	}()
}

func (e *RenderService) StepTracker(id string, notificationType model.NotificationType, logger LeveledLogger) *StepTracker {
	progressTracker, color := newProgressTracker(id, notificationType)
	e.pw.AppendTracker(progressTracker)

	return &StepTracker{
		id:              id,
		stepType:        notificationType,
		progressTracker: progressTracker,
		color:           color,
		logger:          logger,
	}
}

func (e *RenderService) Logger(opts ...PrinterLoggerOption) LeveledLogger {
	return newPrinterLogger(e.pw, opts...)
}

func newProgressWriter() progress.Writer {
	pw := progress.NewWriter()
	pw.SetAutoStop(false)
	pw.SetTrackerLength(40)
	pw.SetMessageLength(50)
	// pw.SetNumTrackersExpected(15)
	pw.SetSortBy(progress.SortByPercent)
	pw.SetStyle(progress.StyleDefault)
	pw.SetTrackerPosition(progress.PositionRight)
	pw.SetUpdateFrequency(time.Second * 1)
	pw.Style().Colors = progress.StyleColorsExample
	pw.Style().Options.PercentFormat = "%4.2f%%"
	pw.Style().Visibility.ETA = true
	pw.Style().Visibility.ETAOverall = true
	pw.Style().Visibility.Percentage = true
	pw.Style().Visibility.Pinned = false
	pw.Style().Visibility.Speed = true
	pw.Style().Visibility.SpeedOverall = true
	pw.Style().Visibility.Time = true
	pw.Style().Visibility.TrackerOverall = false
	pw.Style().Visibility.Value = true
	pw.Style().Visibility.Pinned = false
	pw.Style().Options.TimeInProgressPrecision = time.Millisecond
	pw.Style().Options.TimeDonePrecision = time.Millisecond

	return pw
}

func newProgressTracker(id string, notificationType model.NotificationType) (*progress.Tracker, *text.Color) {
	var unit progress.Units
	var color text.Color
	switch notificationType {
	case model.DownloadNotification:
		unit = progress.UnitsBytes
		color = text.FgWhite
	case model.UploadNotification:
		unit = progress.UnitsBytes
		color = text.FgGreen
	case model.PGSNotification:
		unit = progress.UnitsBytes
		color = text.FgWhite
	case model.FFMPEGSNotification:
		unit = progress.Units{
			Notation:         "",
			NotationPosition: progress.UnitsNotationPositionBefore,
			Formatter: func(value int64) string {
				return formatNumber(value, map[int64]string{
					1000000000000000: "PFrame",
					1000000000000:    "TFrame",
					1000000000:       "GFrame",
					1000000:          "MFrame",
					1000:             "KFrame",
					0:                "Frame",
				})
			},
		}
		color = text.FgBlue
	}
	progressTracker := &progress.Tracker{
		Message: color.Sprintf("[%s] %s", id, notificationType),
		Total:   0,
		Units:   unit,
	}
	return progressTracker, &color
}

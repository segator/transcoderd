package task

import (
	"fmt"
	"github.com/jedib0t/go-pretty/v6/progress"
	"github.com/jedib0t/go-pretty/v6/text"
	"sync"
	"time"
)

type JobStepType string

const DownloadJobStepType = "Download"
const UploadJobStepType = "Upload"
const EncodeJobStepType = "Encode"
const PGSJobStepType = "PGS"

var (
	unitScales = []int64{
		1000000000000000,
		1000000000000,
		1000000000,
		1000000,
		1000,
	}
)

type ConsoleWorkerPrinter struct {
	pw progress.Writer
	mu sync.RWMutex
}

type TaskTracks struct {
	id              string
	stepType        JobStepType
	progressTracker *progress.Tracker
	printer         *text.Color
}

func NewConsoleWorkerPrinter() *ConsoleWorkerPrinter {
	pw := progress.NewWriter()
	pw.SetAutoStop(false)
	pw.SetTrackerLength(40)
	pw.SetMessageWidth(50)
	//pw.SetNumTrackersExpected(15)
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

	return &ConsoleWorkerPrinter{
		pw: pw,
	}
}
func (C *ConsoleWorkerPrinter) Render() {
	C.pw.Render()
}

func (C *ConsoleWorkerPrinter) AddTask(id string, stepType JobStepType) *TaskTracks {
	C.mu.Lock()
	defer C.mu.Unlock()

	var unit progress.Units
	var printer text.Color
	switch stepType {
	case DownloadJobStepType:
		unit = progress.UnitsBytes
		printer = text.FgWhite
		break
	case UploadJobStepType:
		unit = progress.UnitsBytes
		printer = text.FgGreen
		break
	case PGSJobStepType:
		unit = progress.UnitsBytes
		printer = text.FgWhite
	case EncodeJobStepType:
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
		printer = text.FgBlue
		break
	}
	tracker := &progress.Tracker{
		Message: printer.Sprintf("[%s] %s", id, stepType),
		Total:   0,
		Units:   unit,
	}
	taskTrack := &TaskTracks{
		id:              id,
		stepType:        stepType,
		progressTracker: tracker,
		printer:         &printer,
	}

	C.pw.AppendTracker(tracker)
	return taskTrack
}

func (C *ConsoleWorkerPrinter) Log(msg string, a ...interface{}) {
	C.pw.Log(msg, a...)
}

func (C *ConsoleWorkerPrinter) Warn(msg string, a ...interface{}) {
	C.pw.Log(text.FgHiYellow.Sprintf(msg, a...))
}

func (C *ConsoleWorkerPrinter) Cmd(msg string, a ...interface{}) {
	C.pw.Log(text.FgHiCyan.Sprintf(msg, a...))
}

func (C *ConsoleWorkerPrinter) Error(msg string, a ...interface{}) {
	C.pw.Log(text.FgHiRed.Sprintf(msg, a...))
}

func (C *TaskTracks) SetTotal(total int64) {
	C.progressTracker.UpdateTotal(total)
}

func (C *TaskTracks) ETA() time.Duration {
	return C.progressTracker.ETA()
}

func (C *TaskTracks) PercentDone() float64 {
	return C.progressTracker.PercentDone()
}

func (C *TaskTracks) UpdateValue(value int64) {
	C.progressTracker.SetValue(value)
}

func (C *TaskTracks) Increment64(increment int64) {
	C.progressTracker.Increment(increment)
}
func (C *TaskTracks) Increment(increment int) {
	C.progressTracker.Increment(int64(increment))
}

func (C *TaskTracks) Message(msg string) {
	C.progressTracker.UpdateMessage(C.printer.Sprintf("[%s] %s", C.id, msg))
}

func (C *TaskTracks) ResetMessage() {
	C.progressTracker.UpdateMessage(C.printer.Sprintf("[%s] %s", C.id, C.stepType))
}

func (C *TaskTracks) Done() {
	C.progressTracker.SetValue(C.progressTracker.Total)
	C.progressTracker.MarkAsDone()
}

func (C *TaskTracks) Error() {
	C.progressTracker.MarkAsErrored()
}

func formatNumber(value int64, notations map[int64]string) string {
	for _, unitScale := range unitScales {
		if value >= unitScale {
			return fmt.Sprintf("%.2f%s", float64(value)/float64(unitScale), notations[unitScale])
		}
	}
	return fmt.Sprintf("%d%s", value, notations[0])
}

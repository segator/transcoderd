package console

import (
	"fmt"
	"github.com/jedib0t/go-pretty/v6/progress"
	"github.com/jedib0t/go-pretty/v6/text"
	"time"
	"transcoder/model"
)

type StepTracker struct {
	id              string
	stepType        model.NotificationType
	progressTracker *progress.Tracker
	color           *text.Color
	logger          LeveledLogger
}

func (t *StepTracker) ETA() time.Duration {
	return t.progressTracker.ETA()
}

func (t *StepTracker) PercentDone() float64 {
	return t.progressTracker.PercentDone()
}

func (t *StepTracker) SetTotal(total int64) {
	t.progressTracker.UpdateTotal(total)
}

func (t *StepTracker) UpdateValue(value int64) {
	t.progressTracker.SetValue(value)
}

func (t *StepTracker) Increment(increment int) {
	t.progressTracker.Increment(int64(increment))
}

func (t *StepTracker) Done() {
	t.progressTracker.SetValue(t.progressTracker.Total)
	t.progressTracker.MarkAsDone()
}

func (t *StepTracker) Error() {
	t.progressTracker.MarkAsErrored()
}

func (t *StepTracker) Logger() LeveledLogger {
	return t.logger
}

func formatNumber(value int64, notations map[int64]string) string {
	for _, unitScale := range unitScales {
		if value >= unitScale {
			return fmt.Sprintf("%.2f%s", float64(value)/float64(unitScale), notations[unitScale])
		}
	}
	return fmt.Sprintf("%d%s", value, notations[0])
}

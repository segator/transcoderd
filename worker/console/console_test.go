package console

import (
	"bytes"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/jedib0t/go-pretty/v6/progress"

	"transcoder/model"
)

type synchronizedBuffer struct {
	mu sync.Mutex
	bytes.Buffer
}

func (b *synchronizedBuffer) Write(p []byte) (int, error) {
	b.mu.Lock()
	defer b.mu.Unlock()
	return b.Buffer.Write(p)
}

func (b *synchronizedBuffer) String() string {
	b.mu.Lock()
	defer b.mu.Unlock()
	return b.Buffer.String()
}

func TestProgressTrackerRedrawsActiveLineAndFinishes(t *testing.T) {
	tests := []struct {
		name   string
		finish func(*progress.Tracker)
		want   string
	}{
		{
			name: "completed",
			finish: func(tracker *progress.Tracker) {
				tracker.MarkAsDone()
			},
			want: "done!",
		},
		{
			name: "failed",
			finish: func(tracker *progress.Tracker) {
				tracker.MarkAsErrored()
			},
			want: "fail!",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			writer := newProgressWriter()
			writer.SetUpdateFrequency(5 * time.Millisecond)
			output := &synchronizedBuffer{}
			writer.SetOutputWriter(output)

			tracker, _ := newProgressTracker("test-job", model.FFMPEGSNotification)
			tracker.Total = 100
			writer.AppendTracker(tracker)

			renderDone := make(chan struct{})
			go func() {
				writer.Render()
				close(renderDone)
			}()

			waitForConsoleOutput(t, output, "\x1b[A")
			tracker.SetValue(50)
			tt.finish(tracker)
			waitForConsoleOutput(t, output, tt.want)

			writer.Stop()
			select {
			case <-renderDone:
			case <-time.After(time.Second):
				t.Fatal("progress writer did not stop")
			}

			got := output.String()
			if !strings.Contains(got, "\x1b[A") {
				t.Error("active tracker refreshes did not rewind the terminal cursor")
			}
			if count := strings.Count(got, tt.want); count != 1 {
				t.Errorf("completed tracker marker count = %d, want 1", count)
			}
		})
	}
}

func waitForConsoleOutput(t *testing.T, output *synchronizedBuffer, want string) {
	t.Helper()
	deadline := time.Now().Add(time.Second)
	for time.Now().Before(deadline) {
		if strings.Contains(output.String(), want) {
			return
		}
		time.Sleep(time.Millisecond)
	}
	t.Fatalf("console output did not contain %q", want)
}

func TestFormatNumber(t *testing.T) {
	notations := map[int64]string{
		1000000000000000: "PB",
		1000000000000:    "TB",
		1000000000:       "GB",
		1000000:          "MB",
		1000:             "KB",
		0:                "B",
	}
	tests := []struct {
		name  string
		value int64
		want  string
	}{
		{"Bytes", 500, "500B"},
		{"Kilobytes", 1500, "1.50KB"},
		{"Megabytes", 1500000, "1.50MB"},
		{"Gigabytes", 1500000000, "1.50GB"},
		{"Zero", 0, "0B"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := formatNumber(tt.value, notations)
			if got != tt.want {
				t.Errorf("formatNumber(%d) = %q, want %q", tt.value, got, tt.want)
			}
		})
	}
}
func TestUnitScales(t *testing.T) {
	if len(unitScales) == 0 {
		t.Error("unitScales should not be empty")
	}
	for i := 0; i < len(unitScales)-1; i++ {
		if unitScales[i] <= unitScales[i+1] {
			t.Errorf("unitScales not in descending order at index %d", i)
		}
	}
}

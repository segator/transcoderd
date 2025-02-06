package console

import (
	"fmt"
	"github.com/jedib0t/go-pretty/v6/progress"
	"github.com/jedib0t/go-pretty/v6/text"
	"time"
)

type LeveledLogger interface {
	Errorf(msg string, keysAndValues ...interface{})
	Warnf(msg string, keysAndValues ...interface{})
	Logf(msg string, keysAndValues ...interface{})
	Cmdf(msg string, keysAndValues ...interface{})
}

type PrinterLogger struct {
	pw            progress.Writer
	messagePrefix string
}

type PrinterLoggerOption func(*PrinterLogger)

func WithMessagePrefix(prefix string) PrinterLoggerOption {
	return func(pl *PrinterLogger) {
		pl.messagePrefix = prefix
	}
}

func newPrinterLogger(pw progress.Writer, opts ...PrinterLoggerOption) *PrinterLogger {
	pl := &PrinterLogger{pw: pw}
	for _, opt := range opts {
		opt(pl)
	}

	return pl
}
func (c *PrinterLogger) Logf(msg string, a ...interface{}) {
	c.pw.Log(c.log(msg, a...))
}

func (c *PrinterLogger) Warnf(msg string, a ...interface{}) {
	c.pw.Log(text.FgHiYellow.Sprint(c.log(msg, a...)))
}

func (c *PrinterLogger) Cmdf(msg string, a ...interface{}) {
	c.pw.Log(text.FgHiCyan.Sprint(c.log(msg, a...)))
}

func (c *PrinterLogger) Errorf(msg string, a ...interface{}) {
	c.pw.Log(text.FgHiRed.Sprint(c.log(msg, a...)))
}

func (c *PrinterLogger) log(msg string, a ...interface{}) string {
	timestamp := time.Now().Format("2006-01-02 15:04:05")
	printedMessage := fmt.Sprintf(msg, a...)
	if c.messagePrefix != "" {
		return fmt.Sprintf("%s %s %s", timestamp, c.messagePrefix, printedMessage)
	}
	return printedMessage
}

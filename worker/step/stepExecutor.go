package step

import (
	"transcoder/worker/console"
)

type Tracker interface {
	SetTotal(total int64)
	UpdateValue(value int64)
	Increment(increment int)
	Logger() console.LeveledLogger
}

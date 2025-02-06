package step

import (
	"context"
	"transcoder/model"
	"transcoder/worker/console"
	"transcoder/worker/job"
)

type Tracker interface {
	SetTotal(total int64)
	UpdateValue(value int64)
	Increment(increment int)
	Logger() console.LeveledLogger
}

type ActionsFunc func(jobContext *job.Context) []Action
type ActionExecuteFunc func(ctx context.Context, stepTracker Tracker) error
type ActionJobContextFunc func(jobContext *job.Context)
type ActionOnErrorFunc func(jobContext *job.Context, notificationType model.NotificationType, err error)
type ExecutorOption func(stepExecutor *Executor)
type Executor struct {
	stepChan         chan *job.Context
	notificationType model.NotificationType
	OnJob            ActionJobContextFunc
	OnAdd            ActionJobContextFunc
	OnComplete       ActionJobContextFunc
	OnError          ActionOnErrorFunc
	actionsFunc      ActionsFunc
	parallelRunners  int
}

func (s *Executor) NotificationType() model.NotificationType {
	return s.notificationType
}

func (s *Executor) AddJob(job *job.Context) {
	s.OnAdd(job)
	s.stepChan <- job
}

func (s *Executor) GetJobChan() <-chan *job.Context {
	return s.stepChan
}
func (s *Executor) Actions(jobContext *job.Context) []Action {
	return s.actionsFunc(jobContext)
}

func (s *Executor) Parallel() int {
	return s.parallelRunners
}

func (s *Executor) Stop() {
	close(s.stepChan)
}

type Action struct {
	Execute ActionExecuteFunc
	Id      string
}

func NewStepExecutor(notificationType model.NotificationType, actionsFunc ActionsFunc, options ...ExecutorOption) *Executor {
	stepExecutor := &Executor{
		stepChan:         make(chan *job.Context, 20),
		notificationType: notificationType,
		actionsFunc:      actionsFunc,
		parallelRunners:  1,
		OnAdd:            func(*job.Context) {},
		OnJob:            func(*job.Context) {},
		OnComplete:       func(*job.Context) {},
		OnError:          func(*job.Context, model.NotificationType, error) {},
	}
	for _, option := range options {
		option(stepExecutor)
	}
	return stepExecutor
}

func WithParallelRunners(parallelRunners int) ExecutorOption {
	return func(stepExecutor *Executor) {
		stepExecutor.parallelRunners = parallelRunners
	}
}

func WithOnErrorOpt(onError ActionOnErrorFunc) ExecutorOption {
	return func(stepExecutor *Executor) {
		stepExecutor.OnError = onError
	}
}

func WithOnCompleteOpt(onComplete ActionJobContextFunc) ExecutorOption {
	return func(stepExecutor *Executor) {
		stepExecutor.OnComplete = onComplete
	}
}

func WithOnAdd(onAdd ActionJobContextFunc) ExecutorOption {
	return func(stepExecutor *Executor) {
		stepExecutor.OnAdd = onAdd
	}
}

func WithOnJob(onJob ActionJobContextFunc) ExecutorOption {
	return func(stepExecutor *Executor) {
		stepExecutor.OnJob = onJob
	}
}

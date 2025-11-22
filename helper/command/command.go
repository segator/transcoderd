package command

import (
	"context"
	"errors"
	"fmt"
	"io"
	"os"
	"os/exec"
	"runtime"
	"strings"
	"sync"
)

type ReaderFunc func(buffer []byte, exit bool)
type Option struct {
	PanicOnError bool
	AllowedCodes []int
}
type Command struct {
	Command    string
	Params     []string
	Env        []string
	WorkDir    string
	stdoutFunc ReaderFunc
	stderrFunc ReaderFunc
	buffSize   int
}

func NewAllowedCodesOption(codes ...int) Option {
	return Option{
		AllowedCodes: codes,
	}
}
func NewCommandByString(command string, params string) *Command {
	return NewCommand(command, StringToSlice(params)...)
}
func NewCommand(command string, params ...string) *Command {
	cmd := &Command{
		Command:  command,
		Params:   params,
		Env:      os.Environ(),
		WorkDir:  GetWD(),
		buffSize: 4096,
	}
	return cmd
}

func (c *Command) AddParam(param string) *Command {
	c.Params = append(c.Params, param)
	return c
}

func (c *Command) SetWorkDir(workDir string) *Command {
	c.WorkDir = workDir
	return c
}
func (c *Command) SetEnv(env []string) *Command {
	c.Env = env
	return c
}

func (c *Command) AddEnv(env string) *Command {
	c.Env = append(c.Env, env)
	return c
}

func (c *Command) BuffSize(size int) *Command {
	c.buffSize = size
	return c
}

func (c *Command) SetStdoutFunc(stdoutFunc ReaderFunc) *Command {
	c.stdoutFunc = stdoutFunc
	return c
}

func (c *Command) SetStderrFunc(stderrFunc ReaderFunc) *Command {
	c.stderrFunc = stderrFunc
	return c
}
func (c *Command) Run(opt ...Option) (exitCode int, err error) {
	return c.RunWithContext(context.Background(), opt...)
}

func (c *Command) RunWithContext(ctx context.Context, opt ...Option) (exitCode int, err error) {
	wg := &sync.WaitGroup{}
	defer wg.Wait()
	var cmd *exec.Cmd
	if runtime.GOOS == "windows" {
		cmd = exec.CommandContext(ctx, c.Command, c.Params...)
	} else {
		fullCommand := append([]string{c.Command}, c.Params...)
		cmd = exec.CommandContext(ctx, "nice", append([]string{"-20"}, fullCommand...)...)
	}
	cmd.Env = c.Env
	cmd.Dir = c.WorkDir
	stdout, err := cmd.StdoutPipe()
	if err != nil {
		return
	}
	stderr, err := cmd.StderrPipe()
	if err != nil {
		return
	}
	if err = cmd.Start(); err != nil {
		return -1, err
	}

	wg.Add(2)
	go c.readerStreamProcessor(ctx, wg, stdout, c.stdoutFunc)
	go c.readerStreamProcessor(ctx, wg, stderr, c.stderrFunc)

	err = cmd.Wait()
	if err != nil {
		var msg *exec.ExitError
		if errors.As(err, &msg) {
			exitCode := msg.ExitCode()
			if allowedCodes(opt, exitCode) {
				return exitCode, nil
			}
			if isPanicOpt(opt) {
				panic(err)
			}
			return exitCode, err
		}
	}
	return 0, nil
}

func isPanicOpt(opts []Option) bool {
	for _, opt := range opts {
		if opt.PanicOnError {
			return true
		}
	}
	return false
}
func allowedCodes(opts []Option, exitCode int) bool {
	for _, opt := range opts {
		if len(opt.AllowedCodes) > 0 {
			for _, code := range opt.AllowedCodes {
				if code == exitCode {
					return true
				}
			}
		}
	}
	return exitCode == 0
}

func (c *Command) readerStreamProcessor(ctx context.Context, wg *sync.WaitGroup, reader io.ReadCloser, callbackFunc ReaderFunc) {
	defer wg.Done()
	buffer := make([]byte, c.buffSize)
loop:
	for {
		select {
		case <-ctx.Done():
			return
		default:
			readed, err := reader.Read(buffer)
			if err != nil {
				if err == io.EOF {
					if callbackFunc != nil {
						callbackFunc(nil, true)
					}
				}
				break loop
			}
			if callbackFunc != nil {
				callbackFunc(buffer[0:readed], false)
			}
		}
	}
}

func (c *Command) GetFullCommand() string {
	return fmt.Sprintf("%s %s", c.Command, strings.Join(c.Params, " "))
}

func GetWD() string {
	path, err := os.Getwd()
	if err != nil {
		panic(err)
	}
	return path
}

func StringToSlice(command string) (output []string) {
	cutDoubleQuote := true
	cutQuote := true
	inLineWord := ""
	for _, c := range command {
		switch {
		case c == ' ' && cutDoubleQuote && cutQuote:
			if len(inLineWord) > 0 {
				switch inLineWord[0] {
				case '\'':
					inLineWord = strings.Trim(inLineWord, "'")
				case '"':
					inLineWord = strings.Trim(inLineWord, "\"")
				}
				output = append(output, inLineWord)
				inLineWord = ""
			}
			continue
		case c == '"':
			cutDoubleQuote = !cutDoubleQuote
		case c == '\'':
			cutQuote = !cutQuote
		}
		inLineWord += string(c)
	}
	output = append(output, inLineWord)
	return output
}

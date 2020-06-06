package helper

import (
	"context"
	"fmt"
	"github.com/avast/retry-go"
	log "github.com/sirupsen/logrus"
	"gopkg.in/vansante/go-ffprobe.v2"
	"io"
	"io/ioutil"
	"math/rand"
	"net/http"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strings"
	"syscall"
	"time"
)

var(
	ValidVideoExtensions = []string{"mp4","m4a","m4v","f4v","f4a","m4b","m4r","f4b","mov ","ogg","oga","ogv","ogx ","wmv","wma","asf ","webm","avi","flv","vob ","mkv"}
	STUNServers = []string{"https://api.ipify.org?format=text","https://ifconfig.me","https://ident.me/","https://myexternalip.com/raw"}
	workingDirectory = filepath.Join(os.TempDir(),"transcoder")
	ffmpegPath=""
	mkvExtractPath=""
)

func ValidExtension(extension string) bool {
	for _,validExtension := range ValidVideoExtensions {
		if extension == validExtension {
			return true
		}
	}
	return false
}

func CheckPath(path string) {
	if !filepath.IsAbs(path){
		log.Panicf("download-path %s must be absolute and ends with /",path)
	}
}

func GetPublicIP() (publicIP string) {

	retry.Do(func() error{
		randomIndex := rand.Intn(len(STUNServers))
		resp, err := http.Get(STUNServers[randomIndex])
		if err != nil {
			return err
		}
		defer resp.Body.Close()
		publicIPBytes, err := ioutil.ReadAll(resp.Body)
		if err != nil {
			return err
		}
		publicIP=string(publicIPBytes)
		return nil
	},retry.Delay(time.Millisecond*100),retry.Attempts(360),retry.LastErrorOnly(true))
	return publicIP
}

type ReaderFunc func(buffer []byte,exit bool)
func ExecuteCommand(ctx context.Context,command string, params ...string) (exitCode int, err error){
	return ExecuteCommandWithFunc(ctx,command,nil,nil,params...)
}

func ExecuteCommandWithFunc(ctx context.Context,command string, stdoutFunc ReaderFunc,stderrFunc ReaderFunc,params ...string) (exitCode int, err error){
	cmd := exec.CommandContext(ctx,command,params...)
	stdout, err := cmd.StdoutPipe()
	if err!=nil {
		return
	}
	stderr, err := cmd.StderrPipe()
	if err!=nil {
		return
	}
	if err= cmd.Start();err!=nil {
		return -1,err
	}

	go readerStreamProcessor(ctx,stdout,stdoutFunc)
	go readerStreamProcessor(ctx,stderr,stderrFunc)

	err = cmd.Wait()
	if err!=nil{
		if msg, ok := err.(*exec.ExitError); ok{ // there is error code
			exitCode :=  msg.Sys().(syscall.WaitStatus).ExitStatus()
			return exitCode,err
		}else{
			return -1,err
		}
	}
	return 0,nil
}

func readerStreamProcessor(ctx context.Context,reader io.ReadCloser,callbackFunc ReaderFunc){
	buffer := make([]byte, 40)
	loop:
	for {
		select{
			case <-ctx.Done():
				return
			default:
				readed, err := reader.Read(buffer)
				if err != nil {
					if err == io.EOF {
						if callbackFunc!=nil {
							callbackFunc(nil, true)
						}
					}
					break loop
				}
				if callbackFunc!=nil {
					callbackFunc(buffer[0:readed], false)
				}
		}
	}
}

func CommandStringToSlice(command string) (output []string) {
	cutDoubleQuote:=true
	cutQuote:=true
	inLineWord:=""
	for _,c := range command {
		if c == ' ' && cutDoubleQuote && cutQuote {
			if len(inLineWord)>0 {
				if inLineWord[0] == '\''{
					inLineWord = strings.Trim(inLineWord,"'")
				}else if inLineWord[0] == '"'{
					inLineWord = strings.Trim(inLineWord,"\"")
				}
				output = append(output,inLineWord)
				inLineWord=""
			}
			continue
		}else if c == '"' {
			cutDoubleQuote=!cutDoubleQuote
		}else if c == '\'' {
			cutQuote=!cutQuote
		}
		inLineWord=inLineWord+string(c)
	}
	output = append(output,inLineWord)
	return output
}

func GetWD() string {
	path, err := os.Getwd()
	if err != nil {
		panic(err)
	}
	return path
}
func GetWorkingDir() string{
	return workingDirectory
}

func CopyFilePath(src, dst string) (int64, error) {
	sourceFileStat, err := os.Stat(src)
	if err != nil {
		return 0, err
	}

	if !sourceFileStat.Mode().IsRegular() {
		return 0, fmt.Errorf("%s is not a regular file", src)
	}

	source, err := os.Open(src)
	if err != nil {
		return 0, err
	}
	defer source.Close()

	destination, err := os.Create(dst)
	if err != nil {
		return 0, err
	}
	defer destination.Close()
	nBytes, err := io.Copy(destination, source)
	return nBytes, err
}
func ExtractStatikFSFile(statikFS http.FileSystem,statikPath string,targetFile string) (string,error){
	ffprobeFi, err := statikFS.Open(statikPath)
	if err != nil {
		panic(err)
	}
	defer ffprobeFi.Close()

	tempPath := GetWorkingDir()
	err = os.MkdirAll(tempPath, os.ModeDir)
	if err != nil {
		return "",err
	}
	ffprobePath := filepath.Join(tempPath, targetFile)
	ffProbeFile, err := os.OpenFile(ffprobePath, os.O_TRUNC|os.O_CREATE|os.O_RDWR, os.ModePerm)
	if err != nil {
		return "",err
	}
	defer ffProbeFile.Close()
	if _, err := io.Copy(ffProbeFile, ffprobeFi); err != nil {
		return "",err
	}
	return ffprobePath,nil
}

func StatikFSFFProbe(statikFS http.FileSystem) error {
	ffprobeFile := "ffprobe"
	if runtime.GOOS == "windows" {
		ffprobeFile = ffprobeFile + ".exe"
	}
	ffprobePath,err := ExtractStatikFSFile(statikFS,fmt.Sprintf("/ffprobe/%s-%s/%s", runtime.GOOS, runtime.GOARCH, ffprobeFile),ffprobeFile)
	if err!=nil {
		return err
	}

	ffprobe.SetFFProbeBinPath(ffprobePath)
	return nil
}

func StatikFSFFmpeg(statikFS http.FileSystem) error {
	ffprobeFile := "ffmpeg"
	if runtime.GOOS == "windows" {
		ffprobeFile = ffprobeFile + ".exe"
	}
	ffmpegPath,err := ExtractStatikFSFile(statikFS,fmt.Sprintf("/ffmpeg/%s-%s/%s", runtime.GOOS, runtime.GOARCH, ffprobeFile),ffprobeFile)
	if err!=nil {
		return err
	}
	setFFmpegPath(ffmpegPath)
	return nil
}

func StatikFSMKVExtract(statikFS http.FileSystem) error {
	ffprobeFile := "mkvextract"
	if runtime.GOOS == "windows" {
		ffprobeFile = ffprobeFile + ".exe"
	}
	ffmpegPath,err := ExtractStatikFSFile(statikFS,fmt.Sprintf("/mkvextract/%s-%s/%s", runtime.GOOS, runtime.GOARCH, ffprobeFile),ffprobeFile)
	if err!=nil {
		return err
	}
	setMKVExtractPath(ffmpegPath)
	return nil
}

func setFFmpegPath(newFFmpegPath string) {
	ffmpegPath= newFFmpegPath
}
func setMKVExtractPath(newMKVExtractPath string) {
	mkvExtractPath= newMKVExtractPath
}

func GetFFmpegPath() string {
	return ffmpegPath
}

func GetMKVExtractPath() string {
	return mkvExtractPath
}
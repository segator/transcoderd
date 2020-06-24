package helper

import (
	"fmt"
	"github.com/avast/retry-go"
	"github.com/rakyll/statik/fs"
	log "github.com/sirupsen/logrus"
	"gopkg.in/vansante/go-ffprobe.v2"
	"io"
	"io/ioutil"
	"math/rand"
	"net/http"
	"os"
	"path/filepath"
	"runtime"
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
func DisembedFile(embedFS http.FileSystem,statikPath string, targetFilePath string) (string,error){
	embededFile, err := embedFS.Open(statikPath)
	if err != nil {
		panic(err)
	}
	defer embededFile.Close()
	if st,_:= embededFile.Stat(); st.IsDir() {
		err:= fs.Walk(embedFS, statikPath,func(path string, info os.FileInfo, err error) error{
			//I Dont have time for recurisve
			if info.IsDir() {
				return nil
			}
			_,err= DisembedFile(embedFS ,fmt.Sprintf("%s/%s",statikPath,info.Name()),info.Name())
			return err
		});
		return GetWorkingDir(),err
	}

	tempPath := GetWorkingDir()
	err = os.MkdirAll(tempPath, os.ModePerm)
	if err != nil {
		return "",err
	}
	targetCopyFile := filepath.Join(tempPath, targetFilePath)
	ffProbeFile, err := os.OpenFile(targetCopyFile, os.O_TRUNC|os.O_CREATE|os.O_RDWR, os.ModePerm)
	if err != nil {
		return "",err
	}
	defer ffProbeFile.Close()
	if _, err := io.Copy(ffProbeFile, embededFile); err != nil {
		return "",err
	}
	return targetCopyFile,nil
}

func DesembedFSFFProbe(embedFS http.FileSystem) error {
	ffprobeFile := "ffprobe"
	if runtime.GOOS == "windows" {
		ffprobeFile = ffprobeFile + ".exe"
	}
	ffprobePath,err := DisembedFile(embedFS,fmt.Sprintf("/ffprobe/%s-%s/%s", runtime.GOOS, runtime.GOARCH, ffprobeFile),ffprobeFile)
	if err!=nil {
		return err
	}

	ffprobe.SetFFProbeBinPath(ffprobePath)
	return nil
}

func DesembedFFmpeg(embedFS http.FileSystem) error {
	ffprobeFile := "ffmpeg"
	if runtime.GOOS == "windows" {
		ffprobeFile = ffprobeFile + ".exe"
	}
	ffmpegPath,err := DisembedFile(embedFS,fmt.Sprintf("/ffmpeg/%s-%s/%s", runtime.GOOS, runtime.GOARCH, ffprobeFile),ffprobeFile)
	if err!=nil {
		return err
	}
	setFFmpegPath(ffmpegPath)
	return nil
}

func DesembedMKVExtract(embedFS http.FileSystem) error {
	mkvExtractFileName := "mkvextract"
	if runtime.GOOS == "windows" {
		mkvExtractFileName = mkvExtractFileName + ".exe"
	}
	DisembedPath,err := DisembedFile(embedFS,fmt.Sprintf("/mkvextract/%s-%s", runtime.GOOS, runtime.GOARCH), mkvExtractFileName)
	if err!=nil {
		return err
	}
	setMKVExtractPath(filepath.Join(DisembedPath, mkvExtractFileName))
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
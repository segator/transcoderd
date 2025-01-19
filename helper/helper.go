package helper

import (
	"github.com/avast/retry-go"
	log "github.com/sirupsen/logrus"
	"io"
	"math/rand"
	"net/http"
	"path/filepath"
	"time"
)

var (
	ValidVideoExtensions = []string{"mp4", "mpg", "m4a", "m4v", "f4v", "f4a", "m4b", "m4r", "f4b", "mov ", "ogg", "oga", "ogv", "ogx ", "wmv", "wma", "asf ", "webm", "avi", "flv", "vob ", "mkv"}
	STUNServers          = []string{"https://api.ipify.org?format=text", "https://ifconfig.me", "https://ident.me/", "https://myexternalip.com/raw"}
	ffmpegPath           = "ffmpeg"
	mkvExtractPath       = "mkvextract"
)

func ValidExtension(extension string) bool {
	for _, validExtension := range ValidVideoExtensions {
		if extension == validExtension {
			return true
		}
	}
	return false
}

func CheckPath(path string) {
	if !filepath.IsAbs(path) {
		log.Panicf("download-path %s must be absolute and ends with /", path)
	}
}

func GetPublicIP() (publicIP string, err error) {
	err = retry.Do(func() error {
		randomIndex := rand.Intn(len(STUNServers))
		resp, err := http.Get(STUNServers[randomIndex])
		if err != nil {
			return err
		}
		defer resp.Body.Close()
		publicIPBytes, err := io.ReadAll(resp.Body)
		if err != nil {
			return err
		}
		publicIP = string(publicIPBytes)
		return nil
	}, retry.Delay(time.Millisecond*100), retry.Attempts(360), retry.LastErrorOnly(true))
	if err != nil {
		return "", err
	}
	return publicIP, nil
}

func GetFFmpegPath() string {
	return ffmpegPath
}

func GetMKVExtractPath() string {
	return mkvExtractPath
}

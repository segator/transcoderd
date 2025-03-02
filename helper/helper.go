package helper

import (
	log "github.com/sirupsen/logrus"
	"path/filepath"
)

var (
	ValidVideoExtensions = []string{"mp4", "mpg", "m4a", "m4v", "f4v", "f4a", "m4b", "m4r", "f4b", "mov ", "ogg", "oga", "ogv", "ogx ", "wmv", "wma", "asf ", "webm", "avi", "flv", "vob ", "mkv"}
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

func GetFFmpegPath() string {
	return ffmpegPath
}

func GetMKVExtractPath() string {
	return mkvExtractPath
}

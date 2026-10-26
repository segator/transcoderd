package helper

import (
	"os"
	"path/filepath"
	"runtime"

	log "github.com/sirupsen/logrus"
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
	return bundledTool("bin/ffmpeg.exe", ffmpegPath)
}

func GetMKVExtractPath() string {
	return bundledTool("bin/mkvextract.exe", mkvExtractPath)
}

func GetFFProbePath() string {
	return bundledTool("bin/ffprobe.exe", "ffprobe")
}

// BundledPath returns a Windows bundle asset independently of the process cwd.
// The fallback preserves existing PATH-based and non-bundle installations.
func BundledPath(relative, fallback string) string {
	return bundledTool(relative, fallback)
}

func bundledTool(relative, fallback string) string {
	if runtime.GOOS != "windows" {
		return fallback
	}
	executable, err := os.Executable()
	if err != nil {
		return fallback
	}
	return bundlePath(filepath.Dir(executable), relative, fallback)
}

func bundlePath(root, relative, fallback string) string {
	path := filepath.Join(root, filepath.FromSlash(relative))
	if _, err := os.Stat(path); err == nil {
		return path
	}
	return fallback
}

package update

import (
	"crypto/sha1"
	"encoding/hex"
	"fmt"
	"github.com/avast/retry-go"
	"io"
	"io/ioutil"
	"net/http"
	"os"
	"runtime"
	"time"
)

var (
	ApplicationFileName string
	updateURL           = "https://github.com/segator/transcoderd/releases/download/wip-master/%s"
)

func GenerateSha1(path string) (string, error) {
	file, err := os.Open(path)
	if err != nil {
		return "", err
	}
	defer file.Close()
	hash := sha1.New()
	if _, err := io.Copy(hash, file); err != nil {
		return "", err
	}
	hashInBytes := hash.Sum(nil)[:20]
	sha1String := hex.EncodeToString(hashInBytes)
	return sha1String, nil
}

func GenerateSha1File(path string) error {
	sha1, err := GenerateSha1(path)
	if err != nil {
		return err
	}
	sha1PathFile := fmt.Sprintf("%s.sha1", path)
	w, err := os.Create(sha1PathFile)
	if err != nil {
		return err
	}
	defer w.Close()

	w.WriteString(sha1)
	return nil
}

func GetGitHubLatestVersion() string {
	var data []byte
	err := retry.Do(func() error {
		downloadSHA1URL := fmt.Sprintf(updateURL+".sha1", ApplicationFileName)
		resp, err := http.Get(downloadSHA1URL)
		if err != nil {
			return err
		}
		defer resp.Body.Close()
		data, err = ioutil.ReadAll(resp.Body)
		if err != nil {
			return err
		}
		return nil
	}, retry.Delay(time.Second*5), retry.Attempts(300), retry.LastErrorOnly(true))
	if err != nil {
		panic(err)
	}
	return string(data)
}

func HashSha1Myself() string {
	sha1, err := GenerateSha1(os.Args[0])
	if err != nil {
		panic(err)
	}
	return sha1
}

func DownloadAppLatestVersion() string {
	// Create the file
	ext := ""
	if runtime.GOOS == "windows" {
		ext = ".exe"
	}
	RunFile, err := ioutil.TempFile("", fmt.Sprintf("transcoderw*%s", ext))

	if err != nil {
		panic(err)
	}
	defer RunFile.Close()

	// Get the data
	resp, err := http.Get(fmt.Sprintf(updateURL, ApplicationFileName))
	if err != nil {
		panic(err)
	}
	defer resp.Body.Close()

	// Check server response
	if resp.StatusCode != http.StatusOK {
		panic(fmt.Errorf("bad status: %s", resp.Status))
	}

	// Writer the body to file
	_, err = io.Copy(RunFile, resp.Body)
	if err != nil {
		panic(err)
	}
	RunFile.Chmod(os.ModePerm)
	return RunFile.Name()
}

func IsApplicationUpToDate() bool {
	//return true
	return HashSha1Myself() == GetGitHubLatestVersion()
}

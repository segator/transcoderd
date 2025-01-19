package update

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"github.com/avast/retry-go"
	"github.com/blang/semver/v4"
	"github.com/minio/selfupdate"
	log "github.com/sirupsen/logrus"
	"github.com/spf13/pflag"
	"io"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"time"
	"transcoder/helper/command"
)

const (
	repoOwner         = "segator"
	repoName          = "transcoderd"
	latestReleasesURL = "https://api.github.com/repos/%s/%s/releases/latest"
	ExitCode          = 5
)

type GitHubRelease struct {
	TagName string        `json:"tag_name"`
	Assets  []GitHubAsset `json:"assets"`
}

type GitHubAsset struct {
	Name               string `json:"name"`
	BrowserDownloadURL string `json:"browser_download_url"`
}

type Updater struct {
	binaryPath     string
	currentVersion semver.Version
	assetName      string
	tmpPath        string
	noUpdates      bool
	lastCheckTime  time.Time
	lastRelease    *GitHubRelease
	checkInterval  *time.Duration
}

func PFlags() {
	pflag.Duration("updateCheckInterval", time.Minute*15, "Check for updates every X duration")
	pflag.Bool("noUpdateMode", false, "DON'T USE THIS FLAG, INTERNAL USE")
	pflag.Bool("noUpdates", false, "Application will not update itself")
}
func NewUpdater(currentVersionString string, assetName string, noUpdates bool, tmpPath string, checkInterval *time.Duration) (*Updater, error) {
	currentVersion, err := semver.Parse(cleanVersion(currentVersionString))
	if err != nil {
		return nil, err
	}

	updater := &Updater{
		currentVersion: currentVersion,
		binaryPath:     os.Args[0],
		assetName:      assetName,
		tmpPath:        tmpPath,
		noUpdates:      noUpdates,
		checkInterval:  checkInterval,
	}
	return updater, nil
}

func (u *Updater) Run(wg *sync.WaitGroup, ctx context.Context) {
	wg.Add(1)
	go func() {
		for {
			latestVersion, newUpdate, err := u.CheckForUpdate()
			if err != nil {
				log.Error(err)
				select {
				case <-time.After(time.Second * 5):
					continue
				case <-ctx.Done():
					return
				}
			}
			if newUpdate {
				if err = u.update(latestVersion); err != nil {
					log.Error(err)
					continue
				}
			}
			u.runApplication(ctx)
		}
	}()
}

func (u *Updater) runApplication(ctx context.Context) {
	arguments := os.Args[1:]
	arguments = append(arguments, "--noUpdateMode")
	ecode, err := command.NewCommand(u.binaryPath, arguments...).
		SetStderrFunc(func(buffer []byte, exit bool) {
			os.Stderr.Write(buffer)
		}).
		SetStdoutFunc(func(buffer []byte, exit bool) {
			os.Stdout.Write(buffer)
		}).RunWithContext(ctx, command.NewAllowedCodesOption(ExitCode))
	if err != nil && !errors.Is(err, context.Canceled) {
		panic(err)
	}
	if ecode != ExitCode {
		os.Exit(ecode)
	}
}

func (u *Updater) CheckForUpdate() (*GitHubRelease, bool, error) {
	if time.Since(u.lastCheckTime) <= *u.checkInterval {
		return u.lastRelease, false, nil
	}
	latestRelease, err := GetGitHubLatestVersion()
	if err != nil {
		return nil, false, err
	}
	u.lastCheckTime = time.Now()
	u.lastRelease = latestRelease

	latestReleaseVersion, err := semver.Parse(cleanVersion(latestRelease.TagName))
	if err != nil {
		return nil, false, err
	}
	l := log.WithFields(log.Fields{
		"currentVersion": u.currentVersion.String(),
		"latestVersion":  latestReleaseVersion.String(),
	})
	if latestReleaseVersion.GT(u.currentVersion) {
		if u.noUpdates {
			l.Warn("Newer version available but updates are disabled")
			return nil, false, nil
		}

		l.Info("Newer version available")
		return latestRelease, true, nil
	}
	l.Debug("No new version available")
	return nil, false, nil
}

func (u *Updater) update(githubRelease *GitHubRelease) error {
	var assetToDownload *GitHubAsset
	for _, asset := range githubRelease.Assets {
		if asset.Name == u.assetName {
			assetToDownload = &asset
			break
		}
	}
	if assetToDownload == nil {
		return fmt.Errorf("no asset found with name %s for release %s", u.assetName, githubRelease.TagName)
	}
	tmpDownloadPath := filepath.Join(u.tmpPath, fmt.Sprintf("%s.new", u.assetName))
	latestReleaseFile, err := os.Create(tmpDownloadPath)
	if err != nil {
		return err
	}
	defer os.Remove(tmpDownloadPath)
	defer latestReleaseFile.Close()
	err = downloadAsset(assetToDownload.BrowserDownloadURL, latestReleaseFile)
	if err != nil {
		return err
	}

	_, err = latestReleaseFile.Seek(0, io.SeekStart)
	if err != nil {
		return err
	}

	return selfupdate.Apply(latestReleaseFile, selfupdate.Options{})
}

func downloadAsset(assetURL string, wc io.Writer) error {
	resp, err := http.Get(assetURL)
	if err != nil {
		return err
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return fmt.Errorf("unexpected status code: %d", resp.StatusCode)
	}

	_, err = io.Copy(wc, resp.Body)
	return err
}

func cleanVersion(version string) string {
	return strings.TrimPrefix(version, "v")
}

func GetGitHubLatestVersion() (*GitHubRelease, error) {
	var latestRelease GitHubRelease

	err := retry.Do(func() error {
		url := fmt.Sprintf(latestReleasesURL, repoOwner, repoName)
		resp, err := http.Get(url)
		if err != nil {
			return err
		}
		defer resp.Body.Close()
		if resp.StatusCode != 200 {
			return fmt.Errorf("unexpected status code: %d", resp.StatusCode)
		}

		body, err := io.ReadAll(resp.Body)
		if err != nil {
			return err
		}

		err = json.Unmarshal(body, &latestRelease)
		if err != nil {
			return err
		}

		return nil
	},
		retry.Delay(time.Second*5),
		retry.Attempts(100),
		retry.LastErrorOnly(true))

	if err != nil {
		return nil, err
	}

	return &latestRelease, nil
}

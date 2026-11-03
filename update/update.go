package update

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"sync"
	"time"

	"github.com/blang/semver/v4"
	"github.com/minio/selfupdate"
	log "github.com/sirupsen/logrus"
	"github.com/spf13/pflag"

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
	binaryPath         string
	currentVersion     semver.Version
	assetName          string
	tmpPath            string
	noUpdates          bool
	lastCheckTime      time.Time
	lastRelease        *GitHubRelease
	checkInterval      *time.Duration
	latestReleaseCheck func() (*GitHubRelease, error)
}

func PFlags() {
	pflag.Duration("updateCheckInterval", time.Minute*15, "Check for updates every X duration")
	pflag.Bool("noUpdateMode", false, "DON'T USE THIS FLAG, INTERNAL USE")
	pflag.Bool("noUpdates", false, "Application will not update itself")
}

// NewNoopUpdater creates an Updater that never checks for updates.
// Useful for integration tests where network calls are undesirable.
func NewNoopUpdater() *Updater {
	interval := time.Hour * 87600
	return &Updater{
		currentVersion: semver.MustParse("99.99.99"),
		assetName:      "noop",
		noUpdates:      true,
		checkInterval:  &interval,
		lastCheckTime:  time.Now(),
	}
}

func NewUpdater(currentVersionString string, assetName string, noUpdates bool, tmpPath string, checkInterval *time.Duration) (*Updater, error) {
	currentVersion, err := semver.Parse(cleanVersion(currentVersionString))
	if err != nil {
		return nil, err
	}
	binaryPath, err := filepath.Abs(os.Args[0])
	if err != nil {
		return nil, fmt.Errorf("resolve executable path: %w", err)
	}
	if !noUpdates {
		cleanupBinaryBackups(binaryPath)
	}

	updater := &Updater{
		currentVersion: currentVersion,
		binaryPath:     binaryPath,
		assetName:      platformAssetName(assetName, runtime.GOOS, runtime.GOARCH),
		tmpPath:        tmpPath,
		noUpdates:      noUpdates,
		checkInterval:  checkInterval,
	}
	return updater, nil
}

func platformAssetName(application, goos, goarch string) string {
	if goos != "windows" {
		return application
	}
	return application + "-windows-" + goarch + ".exe"
}

func (u *Updater) Run(wg *sync.WaitGroup, ctx context.Context) {
	wg.Add(1)
	go func() {
		defer wg.Done()
		for ctx.Err() == nil {
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
			exitCode, err := u.runApplication(ctx)
			if err != nil {
				panic(err)
			}
			if ctx.Err() != nil {
				return
			}
			if exitCode == ExitCode {
				u.invalidateReleaseCheck()
			}
		}
	}()
}

func (u *Updater) runApplication(ctx context.Context) (int, error) {
	arguments := os.Args[1:]
	arguments = append(arguments, "--noUpdateMode")
	ecode, err := command.NewCommand(u.binaryPath, arguments...).
		SetStderrFunc(func(buffer []byte, exit bool) {
			os.Stderr.Write(buffer)
		}).
		SetStdoutFunc(func(buffer []byte, exit bool) {
			os.Stdout.Write(buffer)
		}).RunWithContext(ctx, command.NewAllowedCodesOption(ExitCode))
	if err != nil {
		if ctx.Err() != nil || errors.Is(err, context.Canceled) {
			return 0, nil
		}
		return 0, err
	}
	if ecode != ExitCode {
		os.Exit(ecode)
	}
	return ecode, nil
}

func (u *Updater) invalidateReleaseCheck() {
	u.lastCheckTime = time.Time{}
	u.lastRelease = nil
}

func (u *Updater) CheckForUpdate() (*GitHubRelease, bool, error) {
	if u.noUpdates {
		return nil, false, nil
	}
	if time.Since(u.lastCheckTime) <= *u.checkInterval {
		return u.lastRelease, false, nil
	}
	// Cache failed checks for the normal interval too. Otherwise every worker
	// poll retries GitHub's API and can keep the worker from accepting jobs when
	// the unauthenticated API rate limit is exhausted.
	u.lastCheckTime = time.Now()
	checkLatestRelease := u.latestReleaseCheck
	if checkLatestRelease == nil {
		checkLatestRelease = GetGitHubLatestVersion
	}
	latestRelease, err := checkLatestRelease()
	if err != nil {
		log.WithError(err).Warn("Unable to check for updates; continuing with the current version")
		return nil, false, nil
	}
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

	return applyBinaryUpdate(u.binaryPath, latestReleaseFile)
}

// applyBinaryUpdate always targets the canonical executable path. On Linux,
// resolving the currently running executable can return its renamed .old path
// after an in-place update, causing subsequent updates to target nested .old
// files instead of the executable started by the user.
func applyBinaryUpdate(binaryPath string, update io.Reader) error {
	options, err := binaryUpdateOptions(binaryPath)
	if err != nil {
		return fmt.Errorf("prepare binary update: %w", err)
	}
	if err := selfupdate.Apply(update, options); err != nil {
		return err
	}
	cleanupBinaryBackups(binaryPath)
	return nil
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
	url := fmt.Sprintf(latestReleasesURL, repoOwner, repoName)
	client := &http.Client{Timeout: 15 * time.Second}
	return getGitHubLatestVersion(client, url)
}

func getGitHubLatestVersion(client *http.Client, url string) (*GitHubRelease, error) {
	request, err := http.NewRequest(http.MethodGet, url, nil)
	if err != nil {
		return nil, err
	}
	request.Header.Set("Accept", "application/vnd.github+json")

	response, err := client.Do(request)
	if err != nil {
		return nil, err
	}
	defer response.Body.Close()
	if response.StatusCode != http.StatusOK {
		body, _ := io.ReadAll(io.LimitReader(response.Body, 4096))
		if len(body) == 0 {
			return nil, fmt.Errorf("unexpected status code: %d", response.StatusCode)
		}
		return nil, fmt.Errorf("unexpected status code: %d: %s", response.StatusCode, strings.TrimSpace(string(body)))
	}

	var latestRelease GitHubRelease
	if err := json.NewDecoder(response.Body).Decode(&latestRelease); err != nil {
		return nil, err
	}

	return &latestRelease, nil
}

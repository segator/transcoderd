package task

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"github.com/avast/retry-go"
	log "github.com/sirupsen/logrus"
	"gopkg.in/ini.v1"
	"gopkg.in/vansante/go-ffprobe.v2"
	"hash"
	"io"
	"mime"
	"net/http"
	"os"
	"path/filepath"
	"regexp"
	"runtime"
	"strconv"
	"strings"
	"sync"
	"sync/atomic"
	"time"
	"transcoder/helper"
	"transcoder/helper/command"
	"transcoder/model"
	"transcoder/worker/config"
	"transcoder/worker/serverclient"
)

const maxPrefetchedJobs = 1

var ErrorJobNotFound = errors.New("job Not found")

type FFMPEGProgress struct {
	duration int
	percent  float64
}
type EncodeWorker struct {
	name         string
	prefetchJobs uint32
	downloadChan chan *model.WorkTaskEncode
	encodeChan   chan *model.WorkTaskEncode
	uploadChan   chan *model.WorkTaskEncode
	workerConfig *config.Config
	tempPath     string
	wg           sync.WaitGroup
	mu           sync.RWMutex
	terminal     *ConsoleWorkerPrinter
	pgsWorker    *PGSWorker
	client       *serverclient.ServerClient
}

func NewEncodeWorker(workerConfig *config.Config, client *serverclient.ServerClient, printer *ConsoleWorkerPrinter) *EncodeWorker {
	tempPath := filepath.Join(workerConfig.TemporalPath, fmt.Sprintf("worker-%s", workerConfig.Name))
	encodeWorker := &EncodeWorker{
		name:         workerConfig.Name,
		pgsWorker:    NewPGSWorker(workerConfig),
		client:       client,
		wg:           sync.WaitGroup{},
		workerConfig: workerConfig,
		downloadChan: make(chan *model.WorkTaskEncode, 100),
		encodeChan:   make(chan *model.WorkTaskEncode, 100),
		uploadChan:   make(chan *model.WorkTaskEncode, 100),
		tempPath:     tempPath,
		terminal:     printer,
		prefetchJobs: 0,
	}
	if err := os.MkdirAll(tempPath, os.ModePerm); err != nil {
		log.Fatal(err)
	}

	return encodeWorker
}

func (e *EncodeWorker) Run(wg *sync.WaitGroup, ctx context.Context) {
	serviceCtx, cancelServiceCtx := context.WithCancel(context.Background())
	log.Info("Starting Worker Client...")
	e.start(serviceCtx)
	log.Info("Started Worker Client...")
	wg.Add(1)
	go func() {
		<-ctx.Done()
		cancelServiceCtx()
		e.stop()
		log.Info("Stopping Worker Client...")
		wg.Done()
	}()
}

func (e *EncodeWorker) start(ctx context.Context) {
	e.resumeJobs()
	go e.terminalRefreshRoutine(ctx)
	go e.downloadQueueRoutine(ctx)
	go e.encodeQueueRoutine(ctx)
	go e.uploadQueueRoutine(ctx)
}

func (e *EncodeWorker) stop() {
	e.terminal.Stop()
	defer close(e.downloadChan)
	defer close(e.uploadChan)
	defer close(e.encodeChan)
}
func (e *EncodeWorker) terminalRefreshRoutine(ctx context.Context) {
	e.wg.Add(1)
	e.terminal.Render()
	<-ctx.Done()
	e.terminal.Stop()
	e.wg.Done()
}

func (e *EncodeWorker) resumeJobs() {
	err := filepath.Walk(e.tempPath, func(path string, info os.FileInfo, err error) error {
		if err != nil {
			return err
		}
		if info.IsDir() {
			return nil
		}
		if filepath.Ext(path) == ".json" {
			filepath.Base(path)
			taskEncode := e.readTaskStatusFromDiskByPath(path)

			if taskEncode.LastState.IsDownloading() {
				e.AddDownloadJob(taskEncode.Task)
				return nil
			}
			if taskEncode.LastState.IsEncoding() {
				// add as prefetched job so won't try to download more jobs until jobs are in encoding phase
				atomic.AddUint32(&e.prefetchJobs, 1)
				t := e.terminal.AddTask(fmt.Sprintf("CACHED: %s", taskEncode.Task.TaskEncode.Id.String()), DownloadJobStepType)
				t.Done()
				e.AddEncodeJob(taskEncode.Task)
				return nil
			}
			if taskEncode.LastState.IsUploading() {
				t := e.terminal.AddTask(fmt.Sprintf("CACHED: %s", taskEncode.Task.TaskEncode.Id.String()), EncodeJobStepType)
				t.Done()
				e.AddUploadJob(taskEncode.Task)
				return nil
			}
		}

		return nil
	})

	if err != nil {
		panic(err)
	}
}

func (e *EncodeWorker) AcceptJobs() bool {
	if e.workerConfig.Paused {
		return false
	}
	if e.workerConfig.HaveSettedPeriodTime() {
		now := time.Now()
		midnight := time.Date(now.Year(), now.Month(), now.Day(), 0, 0, 0, 0, now.Location())
		elapsedSinceMidnight := now.Sub(midnight)
		return elapsedSinceMidnight >= *e.workerConfig.StartAfter && elapsedSinceMidnight <= *e.workerConfig.StopAfter
	}
	return e.PrefetchJobs() < maxPrefetchedJobs
}

func (e *EncodeWorker) dowloadFile(ctx context.Context, job *model.WorkTaskEncode, track *TaskTracks) (err error) {
	err = retry.Do(func() error {
		track.UpdateValue(0)
		req, err := http.NewRequestWithContext(ctx, "GET", e.client.GetDownloadURL(job.TaskEncode.Id), nil)
		if err != nil {
			return err
		}
		req.Header.Set("workerName", e.workerConfig.Name)
		resp, err := http.DefaultClient.Do(req)
		if err != nil {
			return err
		}
		if resp.StatusCode == http.StatusNotFound {
			return ErrorJobNotFound
		}
		if resp.StatusCode != http.StatusOK {
			return fmt.Errorf("not 200 respose in download code %d", resp.StatusCode)
		}
		defer resp.Body.Close()
		size, err := strconv.ParseInt(resp.Header.Get("Content-Length"), 10, 64)
		track.SetTotal(size)
		if err != nil {
			return err
		}
		_, params, err := mime.ParseMediaType(resp.Header.Get("Content-Disposition"))
		if err != nil {
			return err
		}

		job.SourceFilePath = filepath.Join(job.WorkDir, fmt.Sprintf("%s%s", job.TaskEncode.Id.String(), filepath.Ext(params["filename"])))
		dowloadFile, err := os.Create(job.SourceFilePath)
		if err != nil {
			return err
		}

		defer dowloadFile.Close()

		reader := NewProgressTrackStream(track, resp.Body)

		_, err = io.Copy(dowloadFile, reader)
		if err != nil {
			return err
		}
		sha256String := hex.EncodeToString(reader.SumSha())
		bodyString := ""

		err = retry.Do(func() error {
			respsha256, err := http.Get(e.client.GetChecksumURL(job.TaskEncode.Id))
			if err != nil {
				return err
			}
			defer respsha256.Body.Close()
			if respsha256.StatusCode != http.StatusOK {
				return fmt.Errorf("not 200 respose in sha265 code %d", respsha256.StatusCode)
			}

			bodyBytes, err := io.ReadAll(respsha256.Body)
			if err != nil {
				return err
			}
			bodyString = string(bodyBytes)
			return nil
		}, retry.Delay(time.Second*5),
			retry.Attempts(10),
			retry.LastErrorOnly(true),
			retry.OnRetry(func(n uint, err error) {
				e.terminal.Errorf("error %v on calculate checksum of downloaded job", err)
			}),
			retry.RetryIf(func(err error) bool {
				return !errors.Is(err, context.Canceled)
			}))
		if err != nil {
			return err
		}

		if sha256String != bodyString {
			return fmt.Errorf("checksum error on download source:%s downloaded:%s", bodyString, sha256String)
		}

		track.UpdateValue(size)
		return nil
	}, retry.Delay(time.Second*5),
		retry.DelayType(retry.FixedDelay),
		retry.Attempts(180), // 15 min
		retry.LastErrorOnly(true),
		retry.OnRetry(func(n uint, err error) {
			e.terminal.Errorf("Error on downloading job %v", err)
		}),
		retry.RetryIf(func(err error) bool {
			return !(errors.Is(err, context.Canceled) || errors.Is(err, ErrorJobNotFound))
		}))
	return err
}
func (e *EncodeWorker) getVideoParameters(ctx context.Context, inputFile string) (data *ffprobe.ProbeData, size int64, err error) {

	fileReader, err := os.Open(inputFile)
	if err != nil {
		return nil, -1, fmt.Errorf("error opening file %s because %v", inputFile, err)
	}
	stat, err := fileReader.Stat()
	if err != nil {
		return nil, 0, err
	}

	defer fileReader.Close()
	data, err = ffprobe.ProbeReader(ctx, fileReader)
	if err != nil {
		return nil, 0, fmt.Errorf("error getting data: %v", err)
	}
	return data, stat.Size(), nil
}

func FFProbeFrameRate(ffprobeFrameRate string) (frameRate int, err error) {
	rate := 0
	frameRatio := 0
	avgFrameSpl := strings.Split(ffprobeFrameRate, "/")
	if len(avgFrameSpl) != 2 {
		return 0, errors.New("invalid Format")
	}

	frameRatio, err = strconv.Atoi(avgFrameSpl[0])
	if err != nil {
		return 0, err
	}
	rate, err = strconv.Atoi(avgFrameSpl[1])
	if err != nil {
		return 0, err
	}
	return frameRatio / rate, nil
}

func (e *EncodeWorker) clearData(data *ffprobe.ProbeData) (container *ContainerData, err error) {
	container = &ContainerData{}

	videoStream := data.StreamType(ffprobe.StreamVideo)[0]
	frameRate, err := FFProbeFrameRate(videoStream.AvgFrameRate)
	if err != nil {
		frameRate = 24
	}

	container.Video = &Video{
		Id:        uint8(videoStream.Index),
		Duration:  data.Format.Duration(),
		FrameRate: frameRate,
	}

	betterAudioStreamPerLanguage := make(map[string]*Audio)
	for _, stream := range data.StreamType(ffprobe.StreamAudio) {
		if stream.BitRate == "" {
			stream.BitRate = "0"
		}
		bitRateInt, err := strconv.ParseUint(stream.BitRate, 10, 32) // TODO Aqui revem diferents tipos de numeros
		if err != nil {
			panic(err)
		}
		newAudio := &Audio{
			Id:             uint8(stream.Index),
			Language:       stream.Tags.Language,
			Channels:       stream.ChannelLayout,
			ChannelsNumber: uint8(stream.Channels),
			ChannelLayour:  stream.ChannelLayout,
			Default:        stream.Disposition.Default == 1,
			Bitrate:        uint(bitRateInt),
			Title:          stream.Tags.Title,
		}
		betterAudio := betterAudioStreamPerLanguage[newAudio.Language]

		// If more channels or same channels and better bitrate
		if betterAudio != nil {
			if newAudio.ChannelsNumber > betterAudio.ChannelsNumber {
				betterAudioStreamPerLanguage[newAudio.Language] = newAudio
			} else if newAudio.ChannelsNumber == betterAudio.ChannelsNumber && newAudio.Bitrate > betterAudio.Bitrate {
				betterAudioStreamPerLanguage[newAudio.Language] = newAudio
			}
		} else {
			betterAudioStreamPerLanguage[stream.Tags.Language] = newAudio
		}

	}
	for _, audioStream := range betterAudioStreamPerLanguage {
		container.Audios = append(container.Audios, audioStream)
	}

	betterSubtitleStreamPerLanguage := make(map[string]*Subtitle)
	for _, stream := range data.StreamType(ffprobe.StreamSubtitle) {
		newSubtitle := &Subtitle{
			Id:       uint8(stream.Index),
			Language: stream.Tags.Language,
			Forced:   stream.Disposition.Forced == 1,
			Comment:  stream.Disposition.Comment == 1,
			Format:   stream.CodecName,
			Title:    stream.Tags.Title,
		}

		if newSubtitle.Forced || newSubtitle.Comment {
			container.Subtitle = append(container.Subtitle, newSubtitle)
			continue
		}
		// TODO Filter Languages we don't want
		betterSubtitle := betterSubtitleStreamPerLanguage[newSubtitle.Language]
		if betterSubtitle == nil { // TODO Potser perdem subtituls que es necesiten
			betterSubtitleStreamPerLanguage[stream.Tags.Language] = newSubtitle
		} else {
			// TODO aixo es temporal per fer proves, borrar aquest else!!
			container.Subtitle = append(container.Subtitle, newSubtitle)
		}
	}
	for _, value := range betterSubtitleStreamPerLanguage {
		container.Subtitle = append(container.Subtitle, value)
	}
	return container, nil
}
func (e *EncodeWorker) FFMPEG(ctx context.Context, job *model.WorkTaskEncode, videoContainer *ContainerData, ffmpegProgressChan chan<- FFMPEGProgress) error {
	ffmpeg := &FFMPEGGenerator{Config: e.workerConfig.EncodeConfig}
	ffmpeg.setInputFilters(videoContainer, job.SourceFilePath, job.WorkDir)
	ffmpeg.setVideoFilters(videoContainer)
	ffmpeg.setAudioFilters(videoContainer)
	ffmpeg.setSubtFilters(videoContainer)
	ffmpegErrLog := ""

	checkPercentageFFMPEG := func(buffer []byte, exit bool) {
		ffmpegErrLog += string(buffer)
	}

	stdoutFFMPEG := func(buffer []byte, exit bool) {
		cfg, err := ini.Load(buffer)
		if err != nil {
			return
		}
		s := cfg.Section("")
		progress := s.Key("progress").String()
		if progress == "continue" {
			var outTimeSeconds int64
			outTimeUs, err := s.Key("out_time_ms").Int64()
			if err == nil {
				outTimeSeconds = outTimeUs / 1000000
			}
			// If out_time_ms is not present, we can use frame as a fallback, even is not as precise
			if outTimeSeconds == 0 {
				frame, err := s.Key("frame").Int64()
				if err != nil {
					return
				}
				outTimeSeconds = frame / int64(videoContainer.Video.FrameRate)
			}

			ffmpegProgressChan <- FFMPEGProgress{
				duration: int(outTimeSeconds),
				percent:  float64(outTimeSeconds*100) / videoContainer.Video.Duration.Seconds(),
			}
		}
		if exit {
			close(ffmpegProgressChan)
		}
	}
	sourceFileName := filepath.Base(job.SourceFilePath)
	encodedFilePath := fmt.Sprintf("%s-encoded.%s", strings.TrimSuffix(sourceFileName, filepath.Ext(sourceFileName)), "mkv")
	job.TargetFilePath = filepath.Join(job.WorkDir, encodedFilePath)

	ffmpegArguments := ffmpeg.buildArguments(uint8(e.workerConfig.Threads), job.TargetFilePath)
	e.terminal.Cmd("FFMPEG Command:%s %s", helper.GetFFmpegPath(), ffmpegArguments)
	ffmpegCommand := command.NewCommandByString(helper.GetFFmpegPath(), ffmpegArguments).
		SetWorkDir(job.WorkDir).
		SetStdoutFunc(stdoutFFMPEG).
		SetStderrFunc(checkPercentageFFMPEG)

	if runtime.GOOS == "linux" {
		ffmpegCommand.AddEnv(fmt.Sprintf("LD_LIBRARY_PATH=%s", filepath.Dir(helper.GetFFmpegPath())))
	}
	exitCode, err := ffmpegCommand.RunWithContext(ctx)
	if err != nil {
		return fmt.Errorf("%w: stder:%s", err, ffmpegErrLog)
	}
	if exitCode != 0 {
		return fmt.Errorf("exit code %d: stder:%s", exitCode, ffmpegErrLog)
	}

	return nil
}

type ProgressTrackReader struct {
	taskTracker *TaskTracks
	io.ReadCloser
	sha hash.Hash
}

func NewProgressTrackStream(track *TaskTracks, reader io.ReadCloser) *ProgressTrackReader {
	return &ProgressTrackReader{
		taskTracker: track,
		ReadCloser:  reader,
		sha:         sha256.New(),
	}
}

func (p *ProgressTrackReader) Read(b []byte) (n int, err error) {
	n, err = p.ReadCloser.Read(b)
	p.taskTracker.Increment(n)
	p.sha.Write(b[0:n])
	return n, err
}

func (p *ProgressTrackReader) SumSha() []byte {
	return p.sha.Sum(nil)
}

func (e *EncodeWorker) UploadJob(ctx context.Context, task *model.WorkTaskEncode, track *TaskTracks) error {
	e.updateTaskStatus(task, model.UploadNotification, model.StartedNotificationStatus, "")
	err := retry.Do(func() error {
		track.UpdateValue(0)
		encodedFile, err := os.Open(task.TargetFilePath)
		if err != nil {
			return err
		}
		defer encodedFile.Close()
		fi, _ := encodedFile.Stat()
		fileSize := fi.Size()
		track.SetTotal(fileSize)
		sha := sha256.New()
		if _, err = io.Copy(sha, encodedFile); err != nil {
			return err
		}
		checksum := hex.EncodeToString(sha.Sum(nil))
		_, err = encodedFile.Seek(0, io.SeekStart)
		if err != nil {
			return err
		}

		reader := NewProgressTrackStream(track, encodedFile)

		client := &http.Client{}
		req, err := http.NewRequestWithContext(ctx, "POST", e.client.GetUploadURL(task.TaskEncode.Id), reader)
		if err != nil {
			return err
		}
		req.ContentLength = fileSize
		req.Body = reader
		req.GetBody = func() (io.ReadCloser, error) {
			return io.NopCloser(reader), nil
		}
		req.Header.Set("workerName", e.workerConfig.Name)
		req.Header.Add("checksum", checksum)
		req.Header.Add("Content-Type", "application/octet-stream")
		req.Header.Add("Content-Length", strconv.FormatInt(fileSize, 10))
		resp, err := client.Do(req)
		if err != nil {
			return err
		}

		if resp.StatusCode != 201 {
			return fmt.Errorf("invalid status Code %d", resp.StatusCode)
		}
		track.UpdateValue(fileSize)
		return nil
	}, retry.Delay(time.Second*5),
		retry.RetryIf(func(err error) bool {
			return !errors.Is(err, context.Canceled)
		}),
		retry.DelayType(retry.FixedDelay),
		retry.Attempts(17280),
		retry.LastErrorOnly(true),
		retry.OnRetry(func(n uint, err error) {
			e.terminal.Errorf("Error on uploading job %v", err)
		}))

	if err != nil {
		e.updateTaskStatus(task, model.UploadNotification, model.FailedNotificationStatus, "")
		return err
	}

	e.updateTaskStatus(task, model.UploadNotification, model.CompletedNotificationStatus, "")
	return nil
}

func (e *EncodeWorker) errorJob(taskEncode *model.WorkTaskEncode, err error) {
	if errors.Is(err, context.Canceled) {
		e.updateTaskStatus(taskEncode, model.JobNotification, model.CanceledNotificationStatus, "")
	} else {
		e.updateTaskStatus(taskEncode, model.JobNotification, model.FailedNotificationStatus, err.Error())
	}

	err = taskEncode.Clean()
	if err != nil {
		e.terminal.Errorf("Error on cleaning job %s", err.Error())
		return
	}
}

func (e *EncodeWorker) Execute(taskEncode *model.TaskEncode) error {
	workDir := filepath.Join(e.tempPath, taskEncode.Id.String())
	workTaskEncode := &model.WorkTaskEncode{
		TaskEncode: taskEncode,
		WorkDir:    workDir,
	}
	err := os.MkdirAll(workDir, os.ModePerm)
	if err != nil {
		return err
	}

	e.updateTaskStatus(workTaskEncode, model.JobNotification, model.StartedNotificationStatus, "")
	e.AddDownloadJob(workTaskEncode)
	return nil
}

func (e *EncodeWorker) GetID() string {
	return e.name
}
func (e *EncodeWorker) updateTaskStatus(encode *model.WorkTaskEncode, notificationType model.NotificationType, status model.NotificationStatus, message string) {
	e.mu.Lock()
	defer e.mu.Unlock()
	encode.TaskEncode.EventID++
	event := model.TaskEvent{
		Id:               encode.TaskEncode.Id,
		EventID:          encode.TaskEncode.EventID,
		EventType:        model.NotificationEvent,
		WorkerName:       e.workerConfig.Name,
		EventTime:        time.Now(),
		NotificationType: notificationType,
		Status:           status,
		Message:          message,
	}

	if err := e.client.PublishEvent(event); err != nil {
		e.terminal.Errorf("Error on publishing event %s", err.Error())
	}
	if err := e.saveTaskStatusDisk(&model.TaskStatus{
		LastState: &event,
		Task:      encode,
	}); err != nil {
		e.terminal.Errorf("Error on publishing event %s", err.Error())
	}
	e.terminal.Log("[%s] %s have been %s: %s", event.Id.String(), event.NotificationType, event.Status, event.Message)

}

func (e *EncodeWorker) saveTaskStatusDisk(taskEncode *model.TaskStatus) error {
	b, err := json.MarshalIndent(taskEncode, "", "\t")
	if err != nil {
		return err
	}
	eventFile, err := os.OpenFile(filepath.Join(taskEncode.Task.WorkDir, fmt.Sprintf("%s.json", taskEncode.Task.TaskEncode.Id)), os.O_TRUNC|os.O_CREATE|os.O_RDWR, os.ModePerm)
	if err != nil {
		return err
	}
	defer eventFile.Close()
	_, err = eventFile.Write(b)
	if err != nil {
		return err
	}
	return eventFile.Sync()
}
func (e *EncodeWorker) readTaskStatusFromDiskByPath(filepath string) *model.TaskStatus {
	eventFile, err := os.Open(filepath)
	if err != nil {
		panic(err)
	}
	defer eventFile.Close()
	b, err := io.ReadAll(eventFile)
	if err != nil {
		panic(err)
	}
	taskStatus := &model.TaskStatus{}
	err = json.Unmarshal(b, taskStatus)
	if err != nil {
		panic(err)
	}
	return taskStatus
}

func (e *EncodeWorker) PGSMkvExtractDetectAndConvert(ctx context.Context, taskEncode *model.WorkTaskEncode, track *TaskTracks, container *ContainerData) error {
	var PGSTOSrt []*Subtitle
	for _, subt := range container.Subtitle {
		if subt.isImageTypeSubtitle() {
			PGSTOSrt = append(PGSTOSrt, subt)
		}
	}
	if len(PGSTOSrt) > 0 {
		e.updateTaskStatus(taskEncode, model.MKVExtractNotification, model.StartedNotificationStatus, "")
		track.Message(string(model.MKVExtractNotification))
		track.SetTotal(0)
		err := e.MKVExtract(ctx, PGSTOSrt, taskEncode)
		if err != nil {
			e.updateTaskStatus(taskEncode, model.MKVExtractNotification, model.FailedNotificationStatus, err.Error())
			return err
		}
		e.updateTaskStatus(taskEncode, model.MKVExtractNotification, model.CompletedNotificationStatus, "")

		e.updateTaskStatus(taskEncode, model.PGSNotification, model.StartedNotificationStatus, "")
		track.Message(string(model.PGSNotification))
		err = e.convertPGSToSrt(ctx, taskEncode, PGSTOSrt)
		if err != nil {
			e.updateTaskStatus(taskEncode, model.PGSNotification, model.FailedNotificationStatus, err.Error())
			return err
		} else {
			e.updateTaskStatus(taskEncode, model.PGSNotification, model.CompletedNotificationStatus, "")
		}
	}
	return nil
}

func (e *EncodeWorker) convertPGSToSrt(ctx context.Context, taskEncode *model.WorkTaskEncode, subtitles []*Subtitle) error {
	var wg sync.WaitGroup
	tasks := make(chan Subtitle, len(subtitles))
	errs := make(chan error, len(subtitles))
	for i := 0; i < e.workerConfig.PGSConfig.ParallelJobs; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			for subtitle := range tasks {
				pgsTerminalTask := e.terminal.AddTask(fmt.Sprintf("%s %d", taskEncode.TaskEncode.Id.String(), subtitle.Id), PGSJobStepType)
				pgsTerminalTask.SetTotal(0)
				supPath := filepath.Join(taskEncode.WorkDir, fmt.Sprintf("%d.sup", subtitle.Id))
				err := e.pgsWorker.ConvertPGS(ctx, model.TaskPGS{
					PGSID:         int(subtitle.Id),
					PGSSourcePath: supPath,
					PGSTargetPath: filepath.Join(taskEncode.WorkDir, fmt.Sprintf("%d.srt", subtitle.Id)),
					PGSLanguage:   subtitle.Language,
				}, pgsTerminalTask)
				if err != nil {
					pgsTerminalTask.Error()
					errs <- err
				}
				pgsTerminalTask.Done()
			}
		}()
	}

	for _, subtitle := range subtitles {
		tasks <- *subtitle
	}
	close(tasks)
	wg.Wait()
	close(errs)

	var errorList []error
	for err := range errs {
		errorList = append(errorList, err)
	}
	return errors.Join(errorList...)
}

func (e *EncodeWorker) MKVExtract(ctx context.Context, subtitles []*Subtitle, taskEncode *model.WorkTaskEncode) error {
	mkvExtractCommand := command.NewCommand(helper.GetMKVExtractPath(), "tracks", taskEncode.SourceFilePath).
		SetWorkDir(taskEncode.WorkDir)
	if runtime.GOOS == "linux" {
		mkvExtractCommand.AddEnv(fmt.Sprintf("LD_LIBRARY_PATH=%s", filepath.Dir(helper.GetMKVExtractPath())))
	}
	for _, subtitle := range subtitles {
		mkvExtractCommand.AddParam(fmt.Sprintf("%d:%d.sup", subtitle.Id, subtitle.Id))
	}

	_, err := mkvExtractCommand.RunWithContext(ctx, command.NewAllowedCodesOption(0, 1))
	if err != nil {
		e.terminal.Cmd("MKVExtract Command:%s", mkvExtractCommand.GetFullCommand())
		return fmt.Errorf("MKVExtract unexpected error:%v", err)
	}

	return nil
}
func (e *EncodeWorker) PrefetchJobs() uint32 {
	return atomic.LoadUint32(&e.prefetchJobs)
}

func (e *EncodeWorker) AddDownloadJob(job *model.WorkTaskEncode) {
	atomic.AddUint32(&e.prefetchJobs, 1)
	e.downloadChan <- job
}

func (e *EncodeWorker) AddEncodeJob(job *model.WorkTaskEncode) {
	e.encodeChan <- job
}

func (e *EncodeWorker) AddUploadJob(job *model.WorkTaskEncode) {
	e.uploadChan <- job
}

func (e *EncodeWorker) downloadQueueRoutine(ctx context.Context) {
	e.wg.Add(1)
	defer e.wg.Done()
	for {
		select {
		case <-ctx.Done():
			e.terminal.Warn("Stopping Download ServerCoordinator")
			return
		case job, ok := <-e.downloadChan:
			if !ok {
				return
			}
			taskTrack := e.terminal.AddTask(job.TaskEncode.Id.String(), DownloadJobStepType)

			e.updateTaskStatus(job, model.DownloadNotification, model.StartedNotificationStatus, "")
			err := e.dowloadFile(ctx, job, taskTrack)
			if err != nil {
				e.updateTaskStatus(job, model.DownloadNotification, model.FailedNotificationStatus, err.Error())
				taskTrack.Error()
				e.errorJob(job, err)
				atomic.AddUint32(&e.prefetchJobs, ^uint32(0))
				continue
			}
			e.updateTaskStatus(job, model.DownloadNotification, model.CompletedNotificationStatus, "")
			taskTrack.Done()
			e.AddEncodeJob(job)
		}
	}

}

func (e *EncodeWorker) uploadQueueRoutine(ctx context.Context) {
	e.wg.Add(1)
	for {
		select {
		case <-ctx.Done():
			e.terminal.Warn("Stopping Upload ServerCoordinator")
			e.wg.Done()
			return
		case job, ok := <-e.uploadChan:
			if !ok {
				continue
			}
			taskTrack := e.terminal.AddTask(job.TaskEncode.Id.String(), UploadJobStepType)
			err := e.UploadJob(ctx, job, taskTrack)
			if err != nil {
				e.terminal.Errorf("Error on uploading job %v", err)
				taskTrack.Error()
				e.errorJob(job, err)
				continue
			}

			e.updateTaskStatus(job, model.JobNotification, model.CompletedNotificationStatus, "")
			err = job.Clean()
			if err != nil {
				e.terminal.Errorf("Error on cleaning job %v", err)
				taskTrack.Error()
				continue
			}
			taskTrack.Done()
		}
	}

}

func (e *EncodeWorker) encodeQueueRoutine(ctx context.Context) {
	e.wg.Add(1)
	defer e.wg.Done()
	for {
		select {
		case <-ctx.Done():
			e.terminal.Warn("Stopping Encode Queue")
			return
		case job, ok := <-e.encodeChan:
			if !ok {
				return
			}
			atomic.AddUint32(&e.prefetchJobs, ^uint32(0))
			taskTrack := e.terminal.AddTask(job.TaskEncode.Id.String(), EncodeJobStepType)
			err := e.encodeVideo(ctx, job, taskTrack)
			if err != nil {
				taskTrack.Error()
				e.errorJob(job, err)
				continue
			}

			taskTrack.Done()
			e.AddUploadJob(job)
		}
	}

}

func (e *EncodeWorker) encodeVideo(ctx context.Context, job *model.WorkTaskEncode, track *TaskTracks) error {
	e.updateTaskStatus(job, model.FFProbeNotification, model.StartedNotificationStatus, "")
	track.Message(string(model.FFProbeNotification))
	sourceVideoParams, sourceVideoSize, err := e.getVideoParameters(ctx, job.SourceFilePath)
	if err != nil {
		e.updateTaskStatus(job, model.FFProbeNotification, model.FailedNotificationStatus, err.Error())
		return err
	}
	e.updateTaskStatus(job, model.FFProbeNotification, model.CompletedNotificationStatus, "")

	videoContainer, err := e.clearData(sourceVideoParams)
	if err != nil {
		e.terminal.Warn("Error in clearData %s", e.GetID())
		return err
	}
	if err = e.PGSMkvExtractDetectAndConvert(ctx, job, track, videoContainer); err != nil {
		return err
	}
	e.updateTaskStatus(job, model.FFMPEGSNotification, model.StartedNotificationStatus, "")
	track.ResetMessage()
	FFMPEGProgressChan := make(chan FFMPEGProgress)
	go e.FFMPEGProgressRoutine(ctx, job, track, FFMPEGProgressChan, videoContainer)
	err = e.FFMPEG(ctx, job, videoContainer, FFMPEGProgressChan)
	if err != nil {
		e.updateTaskStatus(job, model.FFMPEGSNotification, model.FailedNotificationStatus, err.Error())
		return err
	}
	<-time.After(time.Second * 1)

	if err = e.verifyResultJob(ctx, job, sourceVideoParams, sourceVideoSize); err != nil {
		return err
	}

	e.updateTaskStatus(job, model.FFMPEGSNotification, model.CompletedNotificationStatus, "")
	return nil
}

func (e *EncodeWorker) verifyResultJob(ctx context.Context, job *model.WorkTaskEncode, sourceVideoParams *ffprobe.ProbeData, sourceVideoSize int64) error {
	encodedVideoParams, encodedVideoSize, err := e.getVideoParameters(ctx, job.TargetFilePath)
	if err != nil {
		e.updateTaskStatus(job, model.FFMPEGSNotification, model.FailedNotificationStatus, err.Error())
		return err
	}

	diffDuration := encodedVideoParams.Format.DurationSeconds - sourceVideoParams.Format.DurationSeconds
	if diffDuration > 60 || diffDuration < -60 {
		err = fmt.Errorf("source File duration %f is diferent than encoded %f", sourceVideoParams.Format.DurationSeconds, encodedVideoParams.Format.DurationSeconds)
		//e.updateTaskStatus(job, model.FFMPEGSNotification, model.FailedNotificationStatus, err.Error())
		//return err
	}
	if encodedVideoSize > sourceVideoSize {
		err = fmt.Errorf("source File size %d bytes is less than encoded %d bytes", sourceVideoSize, encodedVideoSize)
		e.updateTaskStatus(job, model.FFMPEGSNotification, model.FailedNotificationStatus, err.Error())
		return err
	}
	return nil
}

func (e *EncodeWorker) FFMPEGProgressRoutine(ctx context.Context, job *model.WorkTaskEncode, track *TaskTracks, ffmpegProgressChan chan FFMPEGProgress, videoContainer *ContainerData) {
	track.SetTotal(int64(videoContainer.Video.Duration.Seconds()) * int64(videoContainer.Video.FrameRate))
	lastProgressEvent := float64(0)

	for {
		select {
		case <-ctx.Done():
			return
		case progress, open := <-ffmpegProgressChan:
			if !open {
				return
			}

			track.UpdateValue(int64(progress.duration * videoContainer.Video.FrameRate))

			if progress.percent-lastProgressEvent > 10 {
				e.updateTaskStatus(job, model.FFMPEGSNotification, model.StartedNotificationStatus, fmt.Sprintf("{\"progress\":\"%.2f\"}", track.PercentDone()))
				lastProgressEvent = progress.percent
			}
		}
	}
}

func (e *EncodeWorker) GetName() string {
	return e.name
}

type FFMPEGGenerator struct {
	Config         *config.FFMPEGConfig
	inputPaths     []string
	VideoFilter    string
	AudioFilter    []string
	SubtitleFilter []string
	Metadata       string
}

func (f *FFMPEGGenerator) setAudioFilters(container *ContainerData) {

	for index, audioStream := range container.Audios {
		// TODO que pasa quan el channelLayout esta empty??
		title := fmt.Sprintf("%s (%s)", audioStream.Language, audioStream.ChannelLayour)
		metadata := fmt.Sprintf(" -metadata:s:a:%d \"title=%s\"", index, title)
		codecQuality := fmt.Sprintf("-c:a:%d %s -vbr %d", index, f.Config.AudioCodec, f.Config.AudioVBR)
		f.AudioFilter = append(f.AudioFilter, fmt.Sprintf(" -map 0:%d %s %s", audioStream.Id, metadata, codecQuality))
	}
}
func (f *FFMPEGGenerator) setVideoFilters(container *ContainerData) {
	videoFilterParameters := "\"scale='min(1920,iw)':-1:force_original_aspect_ratio=decrease\""
	videoEncoderQuality := fmt.Sprintf("-pix_fmt yuv420p10le -c:v %s -crf %d -profile:v %s -preset %s", f.Config.VideoCodec, f.Config.VideoCRF, f.Config.VideoProfile, f.Config.VideoPreset)
	// TODO HDR??
	videoHDR := ""
	f.VideoFilter = fmt.Sprintf("-map 0:%d -avoid_negative_ts make_zero -copyts -map_chapters -1 -flags +global_header -filter:v %s %s %s", container.Video.Id, videoFilterParameters, videoHDR, videoEncoderQuality)

}
func (f *FFMPEGGenerator) setSubtFilters(container *ContainerData) {
	subtInputIndex := 1
	for index, subtitle := range container.Subtitle {
		if subtitle.isImageTypeSubtitle() {
			subtitleMap := fmt.Sprintf("-map %d -c:s:%d srt", subtInputIndex, index)
			subtitleForced := ""
			subtitleComment := ""
			if subtitle.Forced {
				subtitleForced = fmt.Sprintf(" -disposition:s:s:%d forced  -disposition:s:s:%d default", index, index)
			}
			if subtitle.Comment {
				subtitleComment = fmt.Sprintf(" -disposition:s:s:%d comment", index)
			}

			// Clean subtitle title to avoid PGS in title
			re := regexp.MustCompile(`(?i)\(?pgs\)?`)
			subtitleTitle := re.ReplaceAllString(subtitle.Title, "")
			subtitleTitle = strings.TrimSpace(strings.ReplaceAll(subtitleTitle, "  ", " "))

			f.SubtitleFilter = append(f.SubtitleFilter, fmt.Sprintf("%s %s %s -metadata:s:s:%d language=%s -metadata:s:s:%d \"title=%s\" -max_interleave_delta 0", subtitleMap, subtitleForced, subtitleComment, index, subtitle.Language, index, subtitleTitle))
			subtInputIndex++
		} else {
			f.SubtitleFilter = append(f.SubtitleFilter, fmt.Sprintf("-map 0:%d -c:s:%d copy", subtitle.Id, index))
		}

	}
}

func (f *FFMPEGGenerator) buildArguments(threads uint8, outputFilePath string) string {
	coreParameters := fmt.Sprintf("-fflags +genpts -nostats -progress pipe:1  -hide_banner  -threads %d -analyzeduration 2147483647 -probesize 2147483647", threads)
	inputsParameters := ""
	for _, input := range f.inputPaths {
		inputsParameters = fmt.Sprintf("%s -i \"%s\"", inputsParameters, input)
	}
	//-ss 900 -t 10
	audioParameters := ""
	for _, audio := range f.AudioFilter {
		audioParameters = fmt.Sprintf("%s %s", audioParameters, audio)
	}
	subtParameters := ""
	for _, subt := range f.SubtitleFilter {
		subtParameters = fmt.Sprintf("%s %s", subtParameters, subt)
	}

	return fmt.Sprintf("%s %s -max_muxing_queue_size 9999 %s %s %s %s %s -y", coreParameters, inputsParameters, f.VideoFilter, audioParameters, subtParameters, f.Metadata, outputFilePath)
}

func (f *FFMPEGGenerator) setInputFilters(container *ContainerData, sourceFilePath string, workDir string) {
	f.inputPaths = append(f.inputPaths, sourceFilePath)
	if container.HaveImageTypeSubtitle() {
		for _, subt := range container.Subtitle {
			if subt.isImageTypeSubtitle() {
				srtEncodedFile := filepath.Join(workDir, fmt.Sprintf("%d.srt", subt.Id))
				f.inputPaths = append(f.inputPaths, srtEncodedFile)
			}
		}
	}
}

type Video struct {
	Id        uint8
	Duration  time.Duration
	FrameRate int
}
type Audio struct {
	Id             uint8
	Language       string
	Channels       string
	ChannelsNumber uint8
	ChannelLayour  string
	Default        bool
	Bitrate        uint
	Title          string
}
type Subtitle struct {
	Id       uint8
	Language string
	Forced   bool
	Comment  bool
	Format   string
	Title    string
}
type ContainerData struct {
	Video    *Video
	Audios   []*Audio
	Subtitle []*Subtitle
}

func (c *ContainerData) HaveImageTypeSubtitle() bool {
	for _, sub := range c.Subtitle {
		if sub.isImageTypeSubtitle() {
			return true
		}
	}
	return false
}
func (c *ContainerData) ToJson() string {
	b, err := json.Marshal(c)
	if err != nil {
		panic(err)
	}
	return string(b)
}
func (s *Subtitle) isImageTypeSubtitle() bool {
	return strings.Contains(strings.ToLower(s.Format), "pgs")
}

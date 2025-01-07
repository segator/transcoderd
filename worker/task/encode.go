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
	"gopkg.in/vansante/go-ffprobe.v2"
	"hash"
	"io"
	"io/ioutil"
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
	"transcoder/worker/serverclient"
)

const RESET_LINE = "\r\033[K"
const MAX_PREFETCHED_JOBS = 1

var ffmpegSpeedRegex = regexp.MustCompile(`speed=(\d*\.?\d+)x`)
var ErrorJobNotFound = errors.New("job Not found")

type FFMPEGProgress struct {
	duration int
	speed    float64
	percent  float64
}
type EncodeWorker struct {
	name         string
	prefetchJobs uint32
	downloadChan chan *model.WorkTaskEncode
	encodeChan   chan *model.WorkTaskEncode
	uploadChan   chan *model.WorkTaskEncode
	workerConfig Config
	tempPath     string
	wg           sync.WaitGroup
	mu           sync.RWMutex
	terminal     *ConsoleWorkerPrinter
	pgsWorker    *PGSWorker
	client       *serverclient.ServerClient
}

func NewEncodeWorker(workerConfig Config, client *serverclient.ServerClient, printer *ConsoleWorkerPrinter) *EncodeWorker {
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

func (E *EncodeWorker) Run(wg *sync.WaitGroup, ctx context.Context) {
	serviceCtx, cancelServiceCtx := context.WithCancel(context.Background())
	log.Info("Starting WorkerConfig Client...")
	E.start(serviceCtx)
	log.Info("Started WorkerConfig Client...")
	wg.Add(1)
	go func() {
		<-ctx.Done()
		cancelServiceCtx()
		E.stop()
		log.Info("Stopping WorkerConfig Client...")
		wg.Done()
	}()
}

func (E *EncodeWorker) start(ctx context.Context) {
	E.resumeJobs()
	go E.terminalRefreshRoutine(ctx)
	go E.downloadQueueRoutine(ctx)
	go E.encodeQueueRoutine(ctx)
	go E.uploadQueueRoutine(ctx)
}

func (E *EncodeWorker) stop() {
	E.terminal.Stop()
	defer close(E.downloadChan)
	defer close(E.uploadChan)
	defer close(E.encodeChan)
}
func (E *EncodeWorker) terminalRefreshRoutine(ctx context.Context) {
	E.wg.Add(1)
	E.terminal.Render()
	<-ctx.Done()
	E.terminal.Stop()
	E.wg.Done()
}

func (E *EncodeWorker) resumeJobs() {
	err := filepath.Walk(E.tempPath, func(path string, info os.FileInfo, err error) error {
		if err != nil {
			return err
		}
		if info.IsDir() {
			return nil
		}
		if filepath.Ext(path) == ".json" {
			filepath.Base(path)
			taskEncode := E.readTaskStatusFromDiskByPath(path)

			if taskEncode.LastState.IsDownloading() {
				E.AddDownloadJob(taskEncode.Task)
				return nil
			}
			if taskEncode.LastState.IsEncoding() {
				// add as prefetched job so won't try to download more jobs until jobs are in encoding phase
				atomic.AddUint32(&E.prefetchJobs, 1)
				t := E.terminal.AddTask(fmt.Sprintf("CACHED: %s", taskEncode.Task.TaskEncode.Id.String()), DownloadJobStepType)
				t.Done()
				E.AddEncodeJob(taskEncode.Task)
				return nil
			}
			if taskEncode.LastState.IsUploading() {
				t := E.terminal.AddTask(fmt.Sprintf("CACHED: %s", taskEncode.Task.TaskEncode.Id.String()), EncodeJobStepType)
				t.Done()
				E.AddUploadJob(taskEncode.Task)
				return nil
			}
		}

		return nil
	})

	if err != nil {
		panic(err)
	}
}
func durToSec(dur string) (sec int) {
	durAry := strings.Split(dur, ":")
	if len(durAry) != 3 {
		return
	}
	hr, _ := strconv.Atoi(durAry[0])
	sec = hr * (60 * 60)
	min, _ := strconv.Atoi(durAry[1])
	sec += min * (60)
	second, _ := strconv.Atoi(durAry[2])
	sec += second
	return
}
func getSpeed(res string) float64 {
	rs := ffmpegSpeedRegex.FindStringSubmatch(res)
	if len(rs) == 0 {
		return -1
	}
	speed, err := strconv.ParseFloat(rs[1], 64)
	if err != nil {
		return -1
	}
	return speed

}

func getDuration(res string) int {
	i := strings.Index(res, "time=")
	if i >= 0 {
		time := res[i+5:]
		if len(time) > 8 {
			time = time[0:8]
			sec := durToSec(time)
			return sec
		}
	}
	return -1
}

func (J *EncodeWorker) AcceptJobs() bool {
	now := time.Now()
	if J.workerConfig.Paused {
		return false
	}
	if J.workerConfig.HaveSettedPeriodTime() {
		startAfter := time.Date(now.Year(), now.Month(), now.Day(), J.workerConfig.StartAfter.Hour, J.workerConfig.StartAfter.Minute, 0, 0, now.Location())
		stopAfter := time.Date(now.Year(), now.Month(), now.Day(), J.workerConfig.StopAfter.Hour, J.workerConfig.StopAfter.Minute, 0, 0, now.Location())
		return now.After(startAfter) && now.Before(stopAfter)
	}
	return J.PrefetchJobs() < MAX_PREFETCHED_JOBS
}

func (j *EncodeWorker) dowloadFile(job *model.WorkTaskEncode, track *TaskTracks) (err error) {
	err = retry.Do(func() error {

		track.UpdateValue(0)
		resp, err := http.Get(j.client.GetDownloadURL(job.TaskEncode.Id))
		if err != nil {
			return err
		}
		if resp.StatusCode == http.StatusNotFound {
			return ErrorJobNotFound
		}
		if resp.StatusCode != http.StatusOK {
			return fmt.Errorf(fmt.Sprintf("not 200 respose in download code %d", resp.StatusCode))
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
			respsha256, err := http.Get(j.client.GetChecksumURL(job.TaskEncode.Id))
			if err != nil {
				return err
			}
			defer respsha256.Body.Close()
			if respsha256.StatusCode != http.StatusOK {
				return fmt.Errorf(fmt.Sprintf("not 200 respose in sha265 code %d", respsha256.StatusCode))
			}

			bodyBytes, err := ioutil.ReadAll(respsha256.Body)
			if err != nil {
				return err
			}
			bodyString = string(bodyBytes)
			return nil
		}, retry.Delay(time.Second*5),
			retry.Attempts(10),
			retry.LastErrorOnly(true),
			retry.OnRetry(func(n uint, err error) {
				j.terminal.Error("Error %d on calculate checksum of downloaded job %s", err.Error())
			}),
			retry.RetryIf(func(err error) bool {
				return !errors.Is(err, context.Canceled)
			}))
		if err != nil {
			return err
		}

		if sha256String != bodyString {
			return fmt.Errorf("Checksum error on download source:%s downloaded:%s", bodyString, sha256String)
		}

		track.UpdateValue(size)
		return nil
	}, retry.Delay(time.Second*5),
		retry.DelayType(retry.FixedDelay),
		retry.Attempts(180), //15 min
		retry.LastErrorOnly(true),
		retry.OnRetry(func(n uint, err error) {
			j.terminal.Error("Error on downloading job %s", err.Error())
		}),
		retry.RetryIf(func(err error) bool {
			return !(errors.Is(err, context.Canceled) || errors.Is(err, ErrorJobNotFound))
		}))
	return err
}
func (J *EncodeWorker) getVideoParameters(ctx context.Context, inputFile string) (data *ffprobe.ProbeData, size int64, err error) {

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

func FFProbeFrameRate(FFProbeFrameRate string) (frameRate int, err error) {
	rate := 0
	frameRatio := 0
	avgFrameSpl := strings.Split(FFProbeFrameRate, "/")
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

func (j *EncodeWorker) clearData(data *ffprobe.ProbeData) (container *ContainerData, err error) {
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
		bitRateInt, err := strconv.ParseUint(stream.BitRate, 10, 32) //TODO Aqui revem diferents tipos de numeros
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

		//If more channels or same channels and better bitrate
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
		//TODO Filter Languages we don't want
		betterSubtitle := betterSubtitleStreamPerLanguage[newSubtitle.Language]
		if betterSubtitle == nil { //TODO Potser perdem subtituls que es necesiten
			betterSubtitleStreamPerLanguage[stream.Tags.Language] = newSubtitle
		} else {
			//TODO aixo es temporal per fer proves, borrar aquest else!!
			container.Subtitle = append(container.Subtitle, newSubtitle)
		}
	}
	for _, value := range betterSubtitleStreamPerLanguage {
		container.Subtitle = append(container.Subtitle, value)
	}
	return container, nil
}
func (J *EncodeWorker) FFMPEG(ctx context.Context, job *model.WorkTaskEncode, videoContainer *ContainerData, ffmpegProgressChan chan<- FFMPEGProgress) error {
	isClosed := false
	defer func() {
		//close(ffmpegProgressChan)
		isClosed = true
	}()

	ffmpeg := &FFMPEGGenerator{Config: &J.workerConfig.EncodeConfig}
	ffmpeg.setInputFilters(videoContainer, job.SourceFilePath, job.WorkDir)
	ffmpeg.setVideoFilters(videoContainer)
	ffmpeg.setAudioFilters(videoContainer)
	ffmpeg.setSubtFilters(videoContainer, job.WorkDir)
	ffmpeg.setMetadata(videoContainer)
	ffmpegErrLog := ""
	ffmpegOutLog := ""
	sendObj := FFMPEGProgress{
		duration: -1,
		speed:    -1,
	}
	checkPercentageFFMPEG := func(buffer []byte, exit bool) {
		stringedBuffer := string(buffer)
		ffmpegErrLog += stringedBuffer

		duration := getDuration(stringedBuffer)
		if duration != -1 {
			sendObj.duration = duration
			sendObj.percent = float64(duration*100) / videoContainer.Video.Duration.Seconds()

		}
		speed := getSpeed(stringedBuffer)
		if speed != -1 {
			sendObj.speed = speed
		}

		if sendObj.speed != -1 && sendObj.duration != -1 && !isClosed {
			ffmpegProgressChan <- sendObj
			sendObj.duration = -1
			sendObj.speed = -1
		}
	}
	stdoutFFMPEG := func(buffer []byte, exit bool) {
		ffmpegOutLog += string(buffer)
	}
	sourceFileName := filepath.Base(job.SourceFilePath)
	encodedFilePath := fmt.Sprintf("%s-encoded.%s", strings.TrimSuffix(sourceFileName, filepath.Ext(sourceFileName)), "mkv")
	job.TargetFilePath = filepath.Join(job.WorkDir, encodedFilePath)

	ffmpegArguments := ffmpeg.buildArguments(uint8(J.workerConfig.Threads), job.TargetFilePath)
	J.terminal.Cmd("FFMPEG Command:%s %s", helper.GetFFmpegPath(), ffmpegArguments)
	ffmpegCommand := command.NewCommandByString(helper.GetFFmpegPath(), ffmpegArguments).
		SetWorkDir(job.WorkDir).
		SetStdoutFunc(stdoutFFMPEG).
		SetStderrFunc(checkPercentageFFMPEG)

	if runtime.GOOS == "linux" {
		ffmpegCommand.AddEnv(fmt.Sprintf("LD_LIBRARY_PATH=%s", filepath.Dir(helper.GetFFmpegPath())))
	}
	exitCode, err := ffmpegCommand.RunWithContext(ctx)
	if err != nil {
		return fmt.Errorf("%w: stder:%s stdout:%s", err, ffmpegErrLog, ffmpegOutLog)
	}
	if exitCode != 0 {
		return fmt.Errorf("exit code %d: stder:%s stdout:%s", exitCode, ffmpegErrLog, ffmpegOutLog)
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

func (P *ProgressTrackReader) Read(p []byte) (n int, err error) {
	n, err = P.ReadCloser.Read(p)
	P.taskTracker.Increment(n)
	P.sha.Write(p[0:n])
	return n, err
}

func (P *ProgressTrackReader) SumSha() []byte {
	return P.sha.Sum(nil)
}

func (J *EncodeWorker) UploadJob(ctx context.Context, task *model.WorkTaskEncode, track *TaskTracks) error {
	J.updateTaskStatus(task, model.UploadNotification, model.StartedNotificationStatus, "")
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
		if _, err := io.Copy(sha, encodedFile); err != nil {
			return err
		}
		checksum := hex.EncodeToString(sha.Sum(nil))
		encodedFile.Seek(0, io.SeekStart)

		reader := NewProgressTrackStream(track, encodedFile)

		client := &http.Client{}
		//go printProgress(J.ctx, reader, fileSize, wg, "Uploading")
		req, err := http.NewRequestWithContext(ctx, "POST", J.client.GetUploadURL(task.TaskEncode.Id), reader)
		if err != nil {
			return err
		}
		req.ContentLength = fileSize
		req.Body = reader
		req.GetBody = func() (io.ReadCloser, error) {
			return ioutil.NopCloser(reader), nil
		}

		req.Header.Add("checksum", checksum)
		req.Header.Add("Content-Type", "application/octet-stream")
		req.Header.Add("Content-Length", strconv.FormatInt(fileSize, 10))
		resp, err := client.Do(req)

		if err != nil {
			return err
		}
		//wg.Wait()
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
			J.terminal.Error("Error on uploading job %s", err.Error())
		}))

	if err != nil {
		J.updateTaskStatus(task, model.UploadNotification, model.FailedNotificationStatus, "")
		return err
	}

	J.updateTaskStatus(task, model.UploadNotification, model.CompletedNotificationStatus, "")
	return nil
}

func (J *EncodeWorker) errorJob(taskEncode *model.WorkTaskEncode, err error) {
	if errors.Is(err, context.Canceled) {
		J.updateTaskStatus(taskEncode, model.JobNotification, model.CanceledNotificationStatus, "")
	} else {
		J.updateTaskStatus(taskEncode, model.JobNotification, model.FailedNotificationStatus, err.Error())
	}

	taskEncode.Clean()
}

func (J *EncodeWorker) Execute(taskEncode *model.TaskEncode) error {
	workDir := filepath.Join(J.tempPath, taskEncode.Id.String())
	workTaskEncode := &model.WorkTaskEncode{
		TaskEncode: taskEncode,
		WorkDir:    workDir,
	}
	err := os.MkdirAll(workDir, os.ModePerm)
	if err != nil {
		return err
	}

	J.updateTaskStatus(workTaskEncode, model.JobNotification, model.StartedNotificationStatus, "")
	J.AddDownloadJob(workTaskEncode)
	return nil
}

func (J *EncodeWorker) GetID() string {
	return J.name
}
func (J *EncodeWorker) updateTaskStatus(encode *model.WorkTaskEncode, notificationType model.NotificationType, status model.NotificationStatus, message string) {
	J.mu.Lock()
	defer J.mu.Unlock()
	encode.TaskEncode.EventID++
	event := model.TaskEvent{
		Id:               encode.TaskEncode.Id,
		EventID:          encode.TaskEncode.EventID,
		EventType:        model.NotificationEvent,
		WorkerName:       J.workerConfig.Name,
		EventTime:        time.Now(),
		NotificationType: notificationType,
		Status:           status,
		Message:          message,
	}

	if err := J.client.PublishEvent(event); err != nil {
		J.terminal.Error("Error on publishing event %s", err.Error())
	}
	if err := J.saveTaskStatusDisk(&model.TaskStatus{
		LastState: &event,
		Task:      encode,
	}); err != nil {
		J.terminal.Error("Error on publishing event %s", err.Error())
	}
	J.terminal.Log("[%s] %s have been %s: %s", event.Id.String(), event.NotificationType, event.Status, event.Message)

}

func (J *EncodeWorker) saveTaskStatusDisk(taskEncode *model.TaskStatus) error {
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
func (J *EncodeWorker) readTaskStatusFromDiskByPath(filepath string) *model.TaskStatus {
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

func (J *EncodeWorker) PGSMkvExtractDetectAndConvert(ctx context.Context, taskEncode *model.WorkTaskEncode, track *TaskTracks, container *ContainerData) error {
	var PGSTOSrt []*Subtitle
	for _, subt := range container.Subtitle {
		if subt.isImageTypeSubtitle() {
			PGSTOSrt = append(PGSTOSrt, subt)
		}
	}
	if len(PGSTOSrt) > 0 {
		J.updateTaskStatus(taskEncode, model.MKVExtractNotification, model.StartedNotificationStatus, "")
		track.Message(string(model.MKVExtractNotification))
		track.SetTotal(0)
		err := J.MKVExtract(ctx, PGSTOSrt, taskEncode)
		if err != nil {
			J.updateTaskStatus(taskEncode, model.MKVExtractNotification, model.FailedNotificationStatus, err.Error())
			return err
		}
		J.updateTaskStatus(taskEncode, model.MKVExtractNotification, model.CompletedNotificationStatus, "")

		J.updateTaskStatus(taskEncode, model.PGSNotification, model.StartedNotificationStatus, "")
		track.Message(string(model.PGSNotification))
		err = J.convertPGSToSrt(ctx, taskEncode, PGSTOSrt)
		if err != nil {
			J.updateTaskStatus(taskEncode, model.PGSNotification, model.FailedNotificationStatus, err.Error())
			return err
		} else {
			J.updateTaskStatus(taskEncode, model.PGSNotification, model.CompletedNotificationStatus, "")
		}
	}
	return nil
}

func (J *EncodeWorker) convertPGSToSrt(ctx context.Context, taskEncode *model.WorkTaskEncode, subtitles []*Subtitle) error {
	var wg sync.WaitGroup
	tasks := make(chan Subtitle, len(subtitles))
	errs := make(chan error, len(subtitles))
	for i := 0; i < J.workerConfig.PGSConfig.ParallelJobs; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			for subtitle := range tasks {
				pgsTerminalTask := J.terminal.AddTask(fmt.Sprintf("%s %d", taskEncode.TaskEncode.Id.String(), subtitle.Id), PGSJobStepType)
				pgsTerminalTask.SetTotal(0)
				supPath := filepath.Join(taskEncode.WorkDir, fmt.Sprintf("%d.sup", subtitle.Id))
				err := J.pgsWorker.ConvertPGS(ctx, model.TaskPGS{
					PGSID:         int(subtitle.Id),
					PGSSourcePath: supPath,
					PGSTargetPath: filepath.Join(taskEncode.WorkDir, fmt.Sprintf("%d.srt", subtitle.Id)),
					PGSLanguage:   subtitle.Language,
				})
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

func (J *EncodeWorker) MKVExtract(ctx context.Context, subtitles []*Subtitle, taskEncode *model.WorkTaskEncode) error {
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
		J.terminal.Cmd("MKVExtract Command:%s", mkvExtractCommand.GetFullCommand())
		return fmt.Errorf("MKVExtract unexpected error:%v", err.Error())
		return err
	}

	return nil
}
func (J *EncodeWorker) PrefetchJobs() uint32 {
	return atomic.LoadUint32(&J.prefetchJobs)
}

func (J *EncodeWorker) AddDownloadJob(job *model.WorkTaskEncode) {
	atomic.AddUint32(&J.prefetchJobs, 1)
	J.downloadChan <- job
}

func (J *EncodeWorker) AddEncodeJob(job *model.WorkTaskEncode) {
	J.encodeChan <- job
}

func (J *EncodeWorker) AddUploadJob(job *model.WorkTaskEncode) {
	J.uploadChan <- job
}

func (J *EncodeWorker) downloadQueueRoutine(ctx context.Context) {
	J.wg.Add(1)
	defer J.wg.Done()
	for {
		select {
		case <-ctx.Done():
			J.terminal.Warn("Stopping Download ServerCoordinator")
			return
		case job, ok := <-J.downloadChan:
			if !ok {
				return
			}
			taskTrack := J.terminal.AddTask(job.TaskEncode.Id.String(), DownloadJobStepType)

			J.updateTaskStatus(job, model.DownloadNotification, model.StartedNotificationStatus, "")
			err := J.dowloadFile(job, taskTrack)
			if err != nil {
				J.updateTaskStatus(job, model.DownloadNotification, model.FailedNotificationStatus, err.Error())
				taskTrack.Error()
				J.errorJob(job, err)
				atomic.AddUint32(&J.prefetchJobs, ^uint32(0))
				continue
			}
			J.updateTaskStatus(job, model.DownloadNotification, model.CompletedNotificationStatus, "")
			taskTrack.Done()
			J.AddEncodeJob(job)
		}
	}

}

func (J *EncodeWorker) uploadQueueRoutine(ctx context.Context) {
	J.wg.Add(1)
	for {
		select {
		case <-ctx.Done():
			J.terminal.Warn("Stopping Upload ServerCoordinator")
			J.wg.Done()
			return
		case job, ok := <-J.uploadChan:
			if !ok {
				continue
			}
			taskTrack := J.terminal.AddTask(job.TaskEncode.Id.String(), UploadJobStepType)
			err := J.UploadJob(ctx, job, taskTrack)
			if err != nil {
				taskTrack.Error()
				J.errorJob(job, err)
				continue
			}

			J.updateTaskStatus(job, model.JobNotification, model.CompletedNotificationStatus, "")
			taskTrack.Done()
			job.Clean()
		}
	}

}

func (J *EncodeWorker) encodeQueueRoutine(ctx context.Context) {
	J.wg.Add(1)
	defer J.wg.Done()
	for {
		select {
		case <-ctx.Done():
			J.terminal.Warn("Stopping Encode Queue")
			return
		case job, ok := <-J.encodeChan:
			if !ok {
				return
			}
			atomic.AddUint32(&J.prefetchJobs, ^uint32(0))
			taskTrack := J.terminal.AddTask(job.TaskEncode.Id.String(), EncodeJobStepType)
			err := J.encodeVideo(ctx, job, taskTrack)
			if err != nil {
				taskTrack.Error()
				J.errorJob(job, err)
				continue
			}

			taskTrack.Done()
			J.AddUploadJob(job)
		}
	}

}

func (J *EncodeWorker) encodeVideo(ctx context.Context, job *model.WorkTaskEncode, track *TaskTracks) error {
	J.updateTaskStatus(job, model.FFProbeNotification, model.StartedNotificationStatus, "")
	track.Message(string(model.FFProbeNotification))
	sourceVideoParams, sourceVideoSize, err := J.getVideoParameters(ctx, job.SourceFilePath)
	if err != nil {
		J.updateTaskStatus(job, model.FFProbeNotification, model.FailedNotificationStatus, err.Error())
		return err
	}
	J.updateTaskStatus(job, model.FFProbeNotification, model.CompletedNotificationStatus, "")

	videoContainer, err := J.clearData(sourceVideoParams)
	if err != nil {
		J.terminal.Warn("Error in clearData", J.GetID())
		return err
	}
	if err = J.PGSMkvExtractDetectAndConvert(ctx, job, track, videoContainer); err != nil {
		return err
	}
	J.updateTaskStatus(job, model.FFMPEGSNotification, model.StartedNotificationStatus, "")
	track.ResetMessage()
	track.SetTotal(int64(videoContainer.Video.Duration.Seconds()) * int64(videoContainer.Video.FrameRate))
	FFMPEGProgressChan := make(chan FFMPEGProgress)

	go func() {
		lastProgressEvent := float64(0)
		lastDuration := 0
	loop:
		for {
			select {
			case <-ctx.Done():
				return
			case FFMPEGProgress, open := <-FFMPEGProgressChan:
				if !open {
					break loop
				}
				videoContainer.Video.Duration.Seconds()
				encodeFramesIncrement := (FFMPEGProgress.duration - lastDuration) * videoContainer.Video.FrameRate
				lastDuration = FFMPEGProgress.duration

				track.Increment(encodeFramesIncrement)

				if FFMPEGProgress.percent-lastProgressEvent > 10 {
					J.updateTaskStatus(job, model.FFMPEGSNotification, model.StartedNotificationStatus, fmt.Sprintf("{\"progress\":\"%.2f\"}", track.PercentDone()))
					lastProgressEvent = FFMPEGProgress.percent
				}
			}
		}
	}()
	err = J.FFMPEG(ctx, job, videoContainer, FFMPEGProgressChan)
	if err != nil {
		//<-time.After(time.Minute*30)
		J.updateTaskStatus(job, model.FFMPEGSNotification, model.FailedNotificationStatus, err.Error())
		return err
	}
	<-time.After(time.Second * 1)

	encodedVideoParams, encodedVideoSize, err := J.getVideoParameters(ctx, job.TargetFilePath)
	if err != nil {
		J.updateTaskStatus(job, model.FFMPEGSNotification, model.FailedNotificationStatus, err.Error())
		return err
	}

	diffDuration := encodedVideoParams.Format.DurationSeconds - sourceVideoParams.Format.DurationSeconds
	if diffDuration > 60 || diffDuration < -60 {
		err = fmt.Errorf("source File duration %f is diferent than encoded %f", sourceVideoParams.Format.DurationSeconds, encodedVideoParams.Format.DurationSeconds)
		J.updateTaskStatus(job, model.FFMPEGSNotification, model.FailedNotificationStatus, err.Error())
		return err
	}
	if encodedVideoSize > sourceVideoSize {
		err = fmt.Errorf("source File size %d bytes is less than encoded %d bytes", sourceVideoSize, encodedVideoSize)
		J.updateTaskStatus(job, model.FFMPEGSNotification, model.FailedNotificationStatus, err.Error())
		return err
	}
	J.updateTaskStatus(job, model.FFMPEGSNotification, model.CompletedNotificationStatus, "")
	return nil
}

func (E *EncodeWorker) GetName() string {
	return E.name
}

type FFMPEGGenerator struct {
	Config         *FFMPEGConfig
	inputPaths     []string
	VideoFilter    string
	AudioFilter    []string
	SubtitleFilter []string
	Metadata       string
}

func (F *FFMPEGGenerator) setAudioFilters(container *ContainerData) {

	for index, audioStream := range container.Audios {
		//TODO que pasa quan el channelLayout esta empty??
		title := fmt.Sprintf("%s (%s)", audioStream.Language, audioStream.ChannelLayour)
		metadata := fmt.Sprintf(" -metadata:s:a:%d \"title=%s\"", index, title)
		codecQuality := fmt.Sprintf("-c:a:%d %s -vbr %d", index, F.Config.AudioCodec, F.Config.AudioVBR)
		F.AudioFilter = append(F.AudioFilter, fmt.Sprintf(" -map 0:%d %s %s", audioStream.Id, metadata, codecQuality))
	}
}
func (F *FFMPEGGenerator) setVideoFilters(container *ContainerData) {
	videoFilterParameters := "\"scale='min(1920,iw)':-1:force_original_aspect_ratio=decrease\""
	videoEncoderQuality := fmt.Sprintf("-pix_fmt yuv420p10le -c:v %s -crf %d -profile:v %s -preset %s", F.Config.VideoCodec, F.Config.VideoCRF, F.Config.VideoProfile, F.Config.VideoPreset)
	//TODO HDR??
	videoHDR := ""
	F.VideoFilter = fmt.Sprintf("-map 0:%d -avoid_negative_ts make_zero -copyts -map_chapters -1 -flags +global_header -filter:v %s %s %s", container.Video.Id, videoFilterParameters, videoHDR, videoEncoderQuality)

}
func (F *FFMPEGGenerator) setSubtFilters(container *ContainerData, workDir string) {
	subtInputIndex := 1
	for index, subtitle := range container.Subtitle {
		if subtitle.isImageTypeSubtitle() {
			srtEncodedFile := filepath.Join(workDir, fmt.Sprintf("%d.srt", subtitle.Id))
			_, err := os.Stat(srtEncodedFile)
			if os.IsNotExist(err) {
				continue
			}

			subtitleMap := fmt.Sprintf("-map %d -c:s:%d srt", subtInputIndex, index)
			subtitleForced := ""
			subtitleComment := ""
			if subtitle.Forced {
				subtitleForced = fmt.Sprintf(" -disposition:s:s:%d forced  -disposition:s:s:%d default", index, index)
			}
			if subtitle.Comment {
				subtitleComment = fmt.Sprintf(" -disposition:s:s:%d comment", index)
			}

			//Clean subtitle title to avoid PGS in title
			re := regexp.MustCompile(`(?i)pgs`)
			subtitleTitle := re.ReplaceAllString(subtitle.Title, "")
			subtitleTitle = strings.TrimSpace(strings.ReplaceAll(subtitleTitle, "  ", " "))

			F.SubtitleFilter = append(F.SubtitleFilter, fmt.Sprintf("%s %s %s -metadata:s:s:%d language=%s -metadata:s:s:%d \"title=%s\" -max_interleave_delta 0", subtitleMap, subtitleForced, subtitleComment, index, subtitle.Language, index, subtitleTitle))
			subtInputIndex++
		} else {
			F.SubtitleFilter = append(F.SubtitleFilter, fmt.Sprintf("-map 0:%d -c:s:%d copy", subtitle.Id, index))
		}

	}
}
func (F *FFMPEGGenerator) setMetadata(container *ContainerData) {
	F.Metadata = fmt.Sprintf("-metadata encodeParameters='%s'", container.ToJson())
}
func (F *FFMPEGGenerator) buildArguments(threads uint8, outputFilePath string) string {
	coreParameters := fmt.Sprintf("-fflags +genpts -analyzeduration 2147483647 -probesize 2147483647 -hide_banner  -threads %d", threads)
	inputsParameters := ""
	for _, input := range F.inputPaths {
		inputsParameters = fmt.Sprintf("%s -i \"%s\"", inputsParameters, input)
	}
	//-ss 900 -t 10
	audioParameters := ""
	for _, audio := range F.AudioFilter {
		audioParameters = fmt.Sprintf("%s %s", audioParameters, audio)
	}
	subtParameters := ""
	for _, subt := range F.SubtitleFilter {
		subtParameters = fmt.Sprintf("%s %s", subtParameters, subt)
	}

	return fmt.Sprintf("%s %s -max_muxing_queue_size 9999 %s %s %s %s %s -y", coreParameters, inputsParameters, F.VideoFilter, audioParameters, subtParameters, F.Metadata, outputFilePath)
}

func (F *FFMPEGGenerator) setInputFilters(container *ContainerData, sourceFilePath string, workDir string) {
	F.inputPaths = append(F.inputPaths, sourceFilePath)
	if container.HaveImageTypeSubtitle() {
		for _, subt := range container.Subtitle {
			if subt.isImageTypeSubtitle() {
				srtEncodedFile := filepath.Join(workDir, fmt.Sprintf("%d.srt", subt.Id))
				_, err := os.Stat(srtEncodedFile)
				if os.IsNotExist(err) {
					continue
				}
				F.inputPaths = append(F.inputPaths, srtEncodedFile)
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

func (C *ContainerData) HaveImageTypeSubtitle() bool {
	for _, sub := range C.Subtitle {
		if sub.isImageTypeSubtitle() {
			return true
		}
	}
	return false
}
func (C *ContainerData) ToJson() string {
	b, err := json.Marshal(C)
	if err != nil {
		panic(err)
	}
	return string(b)
}
func (C *Subtitle) isImageTypeSubtitle() bool {
	return strings.Index(strings.ToLower(C.Format), "pgs") != -1
}

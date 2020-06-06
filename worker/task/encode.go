package task

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"github.com/avast/retry-go"
	"github.com/google/uuid"
	"github.com/hako/durafmt"
	"github.com/inhies/go-bytesize"
	"gopkg.in/vansante/go-ffprobe.v2"
	"transcoder/helper/progress"

	log "github.com/sirupsen/logrus"
	"io"
	"io/ioutil"
	"mime"
	"net/http"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"sync"
	"time"
	"transcoder/helper"
	"transcoder/model"
)

const RESET_LINE = "\r\033[K"

type EncodeWorker struct {
	model.Manager
	name          string
	ctx           context.Context
	cancelContext context.CancelFunc
	task          model.TaskEncode
	workerConfig  Config
	tempPath      string
}

func NewEncodeWorker(ctx context.Context, workerConfig Config, workerName string) *EncodeWorker {
	newCtx, cancel := context.WithCancel(ctx)
	//strconv.FormatInt(time.Now().Unix(), 10)
	tempPath := filepath.Join(workerConfig.TemporalPath, fmt.Sprintf("worker-%s", workerName))
	jobProcess := &EncodeWorker{
		name:          workerName,
		ctx:           newCtx,
		cancelContext: cancel,
		workerConfig:  workerConfig,
		tempPath:      tempPath,
	}
	return jobProcess
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
func getRatio(res string, duration float64) float64 {
	i := strings.Index(res, "time=")
	if i >= 0 {
		time := res[i+5:]
		if len(time) > 8 {
			time = time[0:8]
			sec := durToSec(time)
			return float64(sec*100.0) / duration
		}
	}
	return -1
}
func printProgress(ctx context.Context, reader *progress.Reader, size int64, wg *sync.WaitGroup, label string) {
	wg.Add(1)
	startTime := time.Now()
	progressChan := progress.NewTicker(ctx, reader, size, 1*time.Second)
	for p := range progressChan {
		blocks := int((p.Percent() / 100) * 50)
		line := "|<"
		for i := 0; i < blocks; i++ {
			line = line + "#"
		}
		for i := 0; i < (50 - blocks); i++ {
			line = line + "-"
		}
		line = line + ">|"
		downloadTime := time.Now().Sub(startTime)
		speed := float64(p.N()) / downloadTime.Seconds()
		fmt.Printf("%s%s %s Speed:%s/s Remaining:%s EstimatedAt: %02d:%02d", RESET_LINE, label, line, bytesize.New(speed), durafmt.Parse(p.Remaining()).String(), p.Estimated().Hour(), p.Estimated().Minute())
	}
	fmt.Printf("\n")
	wg.Done()

}

func (J *EncodeWorker) IsTypeAccepted(jobType string) bool {
	return jobType == string(model.EncodeJobType)
}
func (J *EncodeWorker) Prepare(workData []byte, queueManager model.Manager) error {
	taskEncode := &model.TaskEncode{}
	err := json.Unmarshal(workData, taskEncode)
	if err != nil {
		return err
	}
	if err := os.MkdirAll(J.tempPath, os.ModePerm); err != nil {
		return err
	}
	J.Manager = queueManager
	J.task = *taskEncode
	return nil
}
func (j *EncodeWorker) dowloadFile() (inputfile string, err error) {
	sha := sha256.New()
	err = retry.Do(func() error {

		resp, err := http.Get(j.task.DownloadURL)
		if err != nil {
			return err
		}
		if resp.StatusCode != http.StatusOK {
			return fmt.Errorf(fmt.Sprintf("not 200 respose in dowload code %d", resp.StatusCode))
		}
		defer resp.Body.Close()
		size, err := strconv.ParseInt(resp.Header.Get("Content-Length"), 10, 64)
		if err != nil {
			return err
		}
		_, params, err := mime.ParseMediaType(resp.Header.Get("Content-Disposition"))
		if err != nil {
			return err
		}
		sha = sha256.New()
		inputfile = filepath.Join(j.tempPath, params["filename"])

		dowloadfile, err := os.Create(inputfile)
		if err != nil {
			return err
		}
		defer dowloadfile.Close()

		b := make([]byte, 131072)

		reader := progress.NewReader(resp.Body)
		wg := &sync.WaitGroup{}
		go printProgress(j.ctx, reader, size, wg, "Downloading")

		var readed uint64
	loop:
		for {
			select {
			case <-j.ctx.Done():
				return j.ctx.Err()
			default:
				readedBytes, err := reader.Read(b)
				readed += uint64(readedBytes)
				dowloadfile.Write(b[0:readedBytes])
				sha.Write(b[0:readedBytes])
				//TODO check error here?
				if err == io.EOF {
					break loop
				} else if err != nil {
					return err
				}
			}
		}
		wg.Wait()
		return nil
	}, retry.Delay(time.Second*5), retry.Attempts(10), retry.LastErrorOnly(true), retry.OnRetry(func(n uint, err error) {
		log.Errorf("Error on downloading job %s", err.Error())
	}))
	if err != nil {
		return "", err
	}

	sha256String := hex.EncodeToString(sha.Sum(nil))
	bodyString := ""

	err = retry.Do(func() error {
		respsha256, err := http.Get(j.task.ChecksumURL)
		defer respsha256.Body.Close()
		if err != nil {
			return err
		}
		if respsha256.StatusCode != http.StatusOK {
			return fmt.Errorf(fmt.Sprintf("not 200 respose in sha265 code %d", respsha256.StatusCode))
		}

		bodyBytes, err := ioutil.ReadAll(respsha256.Body)
		if err != nil {
			return err
		}
		bodyString = string(bodyBytes)
		return nil
	}, retry.Delay(time.Second*5), retry.Attempts(10), retry.LastErrorOnly(true), retry.OnRetry(func(n uint, err error) {
		log.Errorf("Error %d on calculate checksum of downloaded job %s", err.Error())
	}))
	if err != nil {
		return "", err
	}

	if sha256String != bodyString {
		return "", err
	}
	return inputfile, nil
}
func (j *EncodeWorker) ffprobeGetData(inputfile string) (data *ffprobe.ProbeData, err error) {

	fileReader, err := os.Open(inputfile)
	if err != nil {
		return nil, fmt.Errorf("error opening test file: %v", err)
	}
	defer fileReader.Close()
	data, err = ffprobe.ProbeReader(j.ctx, fileReader)
	if err != nil {
		return nil, fmt.Errorf("error getting data: %v", err)
	}
	return data, nil
}
func (J *EncodeWorker) UploadJob(err error, encodedFilePath string, sourceFile string) error {
	err = retry.Do(func() error {
		encodedFile, err := os.Open(encodedFilePath)
		if err != nil {
			return err
		}
		defer encodedFile.Close()
		fi, _ := encodedFile.Stat()
		fileSize := fi.Size()
		sha := sha256.New()
		if _, err := io.Copy(sha, encodedFile); err != nil {
			return err
		}
		checksum := hex.EncodeToString(sha.Sum(nil))
		encodedFile.Seek(0, io.SeekStart)
		reader := progress.NewReader(encodedFile)
		wg := &sync.WaitGroup{}
		client := &http.Client{}
		go printProgress(J.ctx, reader, fileSize, wg, "Uploading")
		req, err := http.NewRequestWithContext(J.ctx, "POST", J.task.UploadURL, reader)
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
		resp, err := client.Do(req.WithContext(J.ctx))
		if err != nil {
			return err
		}
		wg.Wait()
		if resp.StatusCode != 201 {
			return fmt.Errorf("invalid status Code %d", resp.StatusCode)
		}
		return nil
	}, retry.Delay(time.Second*5), retry.Attempts(10), retry.LastErrorOnly(true), retry.OnRetry(func(n uint, err error) {
		log.Errorf("Error on uploading job %s", err.Error())
	}))
	return err
}

func (J *EncodeWorker) Execute() (err error) {
	J.sendEvent(model.JobNotification, model.StartedNotificationStatus, "")
	defer func() {
		if r := recover(); r != nil {
			b, _ := json.Marshal(r)
			J.sendEvent(model.JobNotification, model.FailedNotificationStatus, string(b))
			return
		}
		if err != nil {
			if !errors.Is(err, context.Canceled) {
				J.sendEvent(model.JobNotification, model.FailedNotificationStatus, err.Error())
			}
		} else {
			J.sendEvent(model.JobNotification, model.CompletedNotificationStatus, "")
		}
	}()

	J.sendEvent(model.DownloadNotification, model.StartedNotificationStatus, "")
	time.Sleep(time.Second * 2)
	sourceFile, err := J.dowloadFile()
	if err != nil {
		J.sendEvent(model.DownloadNotification, model.FailedNotificationStatus, err.Error())
		return err
	}
	J.sendEvent(model.DownloadNotification, model.CompletedNotificationStatus, "")

	J.sendEvent(model.FFProbeNotification, model.StartedNotificationStatus, "")
	data, err := J.ffprobeGetData(sourceFile)
	if err != nil {
		J.sendEvent(model.FFProbeNotification, model.FailedNotificationStatus, err.Error())
		return err
	}
	J.sendEvent(model.FFProbeNotification, model.CompletedNotificationStatus, "")

	videoContainer, err := J.clearData(data)
	if err != nil {
		return err
	}
	J.sendEvent(model.FFMPEGSNotification, model.StartedNotificationStatus, "")
	FFMPEGProgressChan := make(chan float64)
	go func() {
		lastProgressEvent := 0.0
		startTime := time.Now()
		lastPrintTime := time.Now()
		totalDuration := videoContainer.Video.Duration
	loop:
		for {
			select {
			case <-J.ctx.Done():
				return
			case FFMPEGProgress, open := <-FFMPEGProgressChan:
				if !open {
					break loop
				}
				now := time.Now()
				diffTime := now.Sub(lastPrintTime)
				if diffTime > (time.Second * 1) {
					lastPrintTime = now
					eta := time.Duration(float64(100*(now.Unix()-startTime.Unix()))/FFMPEGProgress) * time.Second
					blocks := int((FFMPEGProgress / 100) * 50)
					line := "|<"
					for i := 0; i < blocks; i++ {
						line = line + "#"
					}
					for i := 0; i < (50 - blocks); i++ {
						line = line + "-"
					}
					line = line + ">|"
					estimatedTime := now.Add(eta)

					speed := totalDuration.Seconds() / estimatedTime.Sub(startTime).Seconds()
					fmt.Printf("\rEncode %s Speed: %.1fx Remaining: %s EstimatedAt: %02d:%02d           ", line, speed, durafmt.Parse(eta).String(), estimatedTime.Hour(), estimatedTime.Minute())
				}

				if FFMPEGProgress-lastProgressEvent > 5 {
					eta := time.Duration(float64(100*(now.Unix()-startTime.Unix()))/FFMPEGProgress) * time.Second
					J.sendEvent(model.FFMPEGSNotification, model.StartedNotificationStatus, fmt.Sprintf("{\"progress\":%.2f,\"remaining\":\"%s\"}", FFMPEGProgress, durafmt.Parse(eta).String()))
					lastProgressEvent = FFMPEGProgress
				}
			}
		}
		fmt.Printf("\n")
	}()
	encodedFilePath, err := J.FFMPEG(sourceFile, videoContainer, FFMPEGProgressChan)
	if err != nil {
		J.sendEvent(model.FFMPEGSNotification, model.FailedNotificationStatus, err.Error())
		return err
	}

	J.sendEvent(model.UploadNotification, model.StartedNotificationStatus, "")
	err = J.UploadJob(err, encodedFilePath, sourceFile)
	if err != nil {
		J.sendEvent(model.UploadNotification, model.FailedNotificationStatus, "")
		return err
	}
	J.sendEvent(model.UploadNotification, model.CompletedNotificationStatus, "")
	return nil
}
func (j *EncodeWorker) clearData(data *ffprobe.ProbeData) (container *ContainerData, err error) {
	container = &ContainerData{}

	videoStream := data.StreamType(ffprobe.StreamVideo)[0]
	container.Video = &Video{
		Id:       uint8(videoStream.Index),
		Duration: data.Format.Duration(),
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
		if strings.Index(strings.ToLower(newSubtitle.Format), "pgs") != -1 {
			return nil, fmt.Errorf("pgs Detected")
		}
		if newSubtitle.Forced || newSubtitle.Comment {
			container.Subtitle = append(container.Subtitle, newSubtitle)
			continue
		}

		betterSubtitle := betterSubtitleStreamPerLanguage[newSubtitle.Language]
		if betterSubtitle == nil { //TODO Potser perdem subtituls!!
			betterSubtitleStreamPerLanguage[stream.Tags.Language] = betterSubtitle
		}
	}
	return container, nil
}
func (J *EncodeWorker) GetTaskID() uuid.UUID {
	return J.task.Id
}
func (J *EncodeWorker) Clean() error {
	log.Warnf("[%s] Cleaning up worker workspace...", J.GetID())
	return os.RemoveAll(J.tempPath)
}
func (J *EncodeWorker) Cancel() {
	log.Warnf("[%s] Canceling job %s...", J.GetID(), J.task.Id.String())
	J.cancelContext()
	//TODO hauriem de esperar de alguna manera a asegurar que el Execute() ha sortit..
	J.sendEvent(model.JobNotification, model.CanceledNotificationStatus, "")
}
func (J *EncodeWorker) GetID() string {
	return J.name
}
func (J *EncodeWorker) sendEvent(notificationType model.NotificationType, status model.NotificationStatus, message string) {
	J.task.EventID++
	event := model.TaskEvent{
		Id:               J.task.Id,
		EventID:          J.task.EventID,
		EventType:        model.NotificationEvent,
		WorkerName:       J.workerConfig.WorkerName,
		EventTime:        time.Now(),
		NotificationType: notificationType,
		Status:           status,
		Message:          message,
	}
	J.Manager.EventNotification(event)
}
func (J *EncodeWorker) FFMPEG(sourceFile string, videoContainer *ContainerData, ffmpegProgressChan chan<- float64) (string, error) {
	ffmpeg := &FFMPEGGenerator{}
	ffmpeg.inputPaths = append(ffmpeg.inputPaths, sourceFile)
	if videoContainer.HaveImageTypeSubtitle() {
		//TODO MKV Extract

		//TODO PGS TO SRT (1 PGS TO SRT per core?)
	}
	ffmpeg.setVideoFilters(videoContainer)
	ffmpeg.setAudioFilters(videoContainer)
	ffmpeg.setSubtFilters(videoContainer)
	ffmpeg.setMetadata(videoContainer)
	ffmpegErrLog := ""
	ffmpegOutLog := ""
	checkPercentageFFMPEG := func(buffer []byte, exit bool) {
		ffmpegErrLog += string(buffer)
		value := getRatio(string(buffer), videoContainer.Video.Duration.Seconds())
		if value != -1 {
			ffmpegProgressChan <- value
		}
	}
	stdoutFFMPEG := func(buffer []byte, exit bool) {
		ffmpegOutLog += string(buffer)
	}
	sourceFileName := filepath.Base(sourceFile)
	encodedFilePath := fmt.Sprintf("%s-encoded.%s", strings.TrimSuffix(sourceFileName, filepath.Ext(sourceFileName)), "mkv")
	outputFullPath := filepath.Join(J.tempPath, encodedFilePath)
	ffmpegArguments := ffmpeg.buildArguments(uint8(J.workerConfig.WorkerThreads), outputFullPath)
	ffmpegArgumentsSlice := helper.CommandStringToSlice(ffmpegArguments)
	exitCode, err := helper.ExecuteCommandWithFunc(J.ctx, "ffmpeg", stdoutFFMPEG, checkPercentageFFMPEG, ffmpegArgumentsSlice...)
	if err != nil {
		return "", fmt.Errorf("%w: stder:%s stdout:%s", err, ffmpegErrLog, ffmpegOutLog)
	}
	if exitCode != 0 {
		return "", fmt.Errorf("exit code %d: stder:%s stdout:%s", exitCode, ffmpegErrLog, ffmpegOutLog)
	}
	close(ffmpegProgressChan)
	return outputFullPath, nil
}

type FFMPEGGenerator struct {
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
		metadata := fmt.Sprintf(" -metadata:s:a:%d title=\"%s\"", index, title)
		codecQuality := fmt.Sprintf("-acodec %s -vbr %d", "aac", 5)
		F.AudioFilter = append(F.AudioFilter, fmt.Sprintf(" -map 0:%d %s %s", audioStream.Id, metadata, codecQuality))
	}
}
func (F *FFMPEGGenerator) setVideoFilters(container *ContainerData) {
	videoFilterParameters := fmt.Sprintf("\"scale='min(%d,iw)':min'(%d,ih)':force_original_aspect_ratio=decrease\"", 1920, 1080)
	videoEncoderQuality := fmt.Sprintf("-c:v %s -crf %d -preset %s", "libx265", 21, "slow")
	//TODO HDR??
	videoHDR := ""
	F.VideoFilter = fmt.Sprintf("-map 0:%d -map_chapters -1 -flags +global_header -filter:v %s %s %s", container.Video.Id, videoFilterParameters, videoHDR, videoEncoderQuality)

}
func (F *FFMPEGGenerator) setSubtFilters(container *ContainerData) {
	subtIndex := 1
	for index, subtitle := range container.Subtitle {
		if subtitle.isImageTypeSubtitle() {
			subtitleMap := fmt.Sprintf("-map %d -c:s srt", subtIndex)
			subtitleForced := ""
			if subtitle.Forced {
				subtitleForced = fmt.Sprintf(" -disposition:s:s:%d forced  -disposition:s:s:%d default", index, index)
			}
			F.SubtitleFilter = append(F.SubtitleFilter, fmt.Sprintf("%s %s -metadata:s:s:%d language=%s -metadata:s:s:%d title=\"%s\"", subtitleMap, subtitleForced, index, subtitle.Language, index, subtitle.Title))
			subtIndex++
		} else {
			F.SubtitleFilter = append(F.SubtitleFilter, fmt.Sprintf("-map 0:%d -c:s copy -sub_charenc UTF-8", subtitle.Id))
		}

	}
}
func (F *FFMPEGGenerator) setMetadata(container *ContainerData) {
	F.Metadata = fmt.Sprintf("-metadata encodeParameters='%s'", container.ToJson())
}
func (F *FFMPEGGenerator) buildArguments(threads uint8, outputFilePath string) string {
	coreParameters := fmt.Sprintf("-hide_banner  -threads %d", threads)
	inputsParameters := ""
	for _, input := range F.inputPaths {
		inputsParameters = fmt.Sprintf("%s -i \"%s\"", inputsParameters, input)
	}
	audioParameters := ""
	for _, audio := range F.AudioFilter {
		audioParameters = fmt.Sprintf("%s %s", audioParameters, audio)
	}
	subtParameters := ""
	for _, subt := range F.SubtitleFilter {
		subtParameters = fmt.Sprintf("%s %s", subtParameters, subt)
	}

	return fmt.Sprintf("%s %s %s %s %s %s %s", coreParameters, inputsParameters, F.VideoFilter, audioParameters, subtParameters, F.Metadata, outputFilePath)
}

type Video struct {
	Id       uint8
	Duration time.Duration
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
	return false
}

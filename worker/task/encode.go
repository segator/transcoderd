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
	log "github.com/sirupsen/logrus"
	"gopkg.in/vansante/go-ffprobe.v2"
	"io"
	"io/ioutil"
	"mime"
	"net/http"
	"os"
	"path/filepath"
	"runtime"
	"strconv"
	"strings"
	"sync"
	"time"
	"transcoder/helper"
	"transcoder/helper/command"
	"transcoder/helper/progress"
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
	tempPath := filepath.Join(workerConfig.TemporalPath, fmt.Sprintf("worker-%s", workerName))
	encodeWorker := &EncodeWorker{
		name:          workerName,
		ctx:           newCtx,
		cancelContext: cancel,
		workerConfig:  workerConfig,
		tempPath:      tempPath,
	}
	return encodeWorker
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
	//TODO no calcula be el temps/velocitat, donat que el calcula desde el principi i hauria de calcular amb els ultims X segons
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
		fmt.Printf("%s%s %s Speed:%s/s Remaining:%s EstimatedAt: %02d:%02d", RESET_LINE, label, line, bytesize.New(speed), durafmt.Parse(p.Remaining()).LimitFirstN(2).String(), p.Estimated().Hour(), p.Estimated().Minute())
	}
	fmt.Printf("\n")
	wg.Done()

}

func (J *EncodeWorker) IsTypeAccepted(jobType string) bool {
	return jobType == string(model.EncodeJobType)
}

func (J *EncodeWorker) AcceptJobs() bool {
	now := time.Now()
	if J.workerConfig.Paused {
		return false
	}
	if J.workerConfig.HaveSettedPeriodTime() {
		startAfter := time.Date(now.Year(),now.Month(),now.Day(),J.workerConfig.StartAfter.Hour,J.workerConfig.StartAfter.Minute,0,0,now.Location())
		stopAfter := time.Date(now.Year(),now.Month(),now.Day(),J.workerConfig.StopAfter.Hour,J.workerConfig.StopAfter.Minute,0,0,now.Location())
		return now.After(startAfter) && now.Before(stopAfter)
	}
	return true
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
		newFileName:=fmt.Sprintf("%s%s",j.GetTaskID().String(),filepath.Ext(params["filename"]))
		sha = sha256.New()
		inputfile = filepath.Join(j.tempPath, newFileName)

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
	}, retry.Delay(time.Second*5),
	retry.Attempts(10),
	retry.LastErrorOnly(true),
	retry.OnRetry(func(n uint, err error) {
		log.Errorf("Error on downloading job %s", err.Error())
	}),
	retry.RetryIf(func(err error) bool {
		return !errors.Is(err,context.Canceled)
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
	}, retry.Delay(time.Second*5),
	retry.Attempts(10),
	retry.LastErrorOnly(true),
	retry.OnRetry(func(n uint, err error) {
		log.Errorf("Error %d on calculate checksum of downloaded job %s", err.Error())
	}),
	retry.RetryIf(func(err error) bool {
		return !errors.Is(err,context.Canceled)
	}))
	if err != nil {
		return "", err
	}

	if sha256String != bodyString {
		return "", err
	}
	return inputfile, nil
}
func (J *EncodeWorker) ffprobeGetData(inputfile string) (data *ffprobe.ProbeData, err error) {

	fileReader, err := os.Open(inputfile)
	if err != nil {
		return nil, fmt.Errorf("error opening test file: %v", err)
	}
	defer fileReader.Close()
	data, err = ffprobe.ProbeReader(J.ctx, fileReader)
	if err != nil {
		return nil, fmt.Errorf("error getting data: %v", err)
	}
	return data, nil
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

		if newSubtitle.Forced || newSubtitle.Comment {
			container.Subtitle = append(container.Subtitle, newSubtitle)
			continue
		}
		//TODO Filter Languages we don't want
		betterSubtitle := betterSubtitleStreamPerLanguage[newSubtitle.Language]
		if betterSubtitle == nil { //TODO Potser perdem subtituls que es necesiten
			betterSubtitleStreamPerLanguage[stream.Tags.Language] = newSubtitle
		}else{
			//TODO aixo es temporal per fer proves, borrar aquest else!!
			container.Subtitle=append(container.Subtitle,newSubtitle)
		}
	}
	for  _, value := range betterSubtitleStreamPerLanguage {
		container.Subtitle=append(container.Subtitle,value)
	}
	return container, nil
}
func (J *EncodeWorker) FFMPEG(sourceFile string, videoContainer *ContainerData, ffmpegProgressChan chan<- float64) (string, error) {
	ffmpeg := &FFMPEGGenerator{}
	ffmpeg.setInputFilters(videoContainer,sourceFile,J.tempPath)
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
	log.Debugf("FFMPEG Command:%s %s",helper.GetFFmpegPath(),ffmpegArguments)
	ffmpegCommand:=command.NewCommandByString(helper.GetFFmpegPath(),ffmpegArguments).
		SetWorkDir(J.tempPath).
		SetStdoutFunc(stdoutFFMPEG).
		SetStderrFunc(checkPercentageFFMPEG)
	exitCode, err := ffmpegCommand.RunWithContext(J.ctx)
	if err != nil {
		return "", fmt.Errorf("%w: stder:%s stdout:%s", err, ffmpegErrLog, ffmpegOutLog)
	}
	if exitCode != 0 {
		return "", fmt.Errorf("exit code %d: stder:%s stdout:%s", exitCode, ffmpegErrLog, ffmpegOutLog)
	}
	close(ffmpegProgressChan)
	return outputFullPath, nil
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
		resp, err := client.Do(req)

		if err != nil {
			return err
		}
		wg.Wait()
		if resp.StatusCode != 201 {
			return fmt.Errorf("invalid status Code %d", resp.StatusCode)
		}
		return nil
	}, retry.Delay(time.Second*5),
	retry.RetryIf(func(err error) bool {
		return !errors.Is(err,context.Canceled)
	}),
	retry.Attempts(10),
	retry.LastErrorOnly(true),
	retry.OnRetry(func(n uint, err error) {
		log.Errorf("Error on uploading job %s", err.Error())
	}))
	return err
}
func (J *EncodeWorker) Clean() error {
	log.Warnf("[%s] Cleaning up worker workspace...", J.GetID())
	err := os.RemoveAll(J.tempPath)
	if err != nil {
		log.Error("error in clean folder", J.GetID())
		return err
	}
	return nil
}

func (J *EncodeWorker) Execute() (err error) {
	J.sendEvent(model.JobNotification, model.StartedNotificationStatus, "")
	defer func() {
		if r := recover(); r != nil {
			switch e := r.(type) {
			case error:
				if errors.Is(e.(error), context.Canceled) {
					J.sendEvent(model.JobNotification, model.CanceledNotificationStatus, "")
				}else{
					J.sendEvent(model.JobNotification, model.FailedNotificationStatus, e.Error())
				}
			default:
				b, _ := json.Marshal(r)
				J.sendEvent(model.JobNotification, model.FailedNotificationStatus, string(b))
			}
			return
		}
		if err != nil {
			if errors.Is(err, context.Canceled) {
				J.sendEvent(model.JobNotification, model.CanceledNotificationStatus, "")
			}else{
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
		log.Error("Error in clearData", J.GetID())
		return err
	}
	if err=J.PGSMkvExtractDetectAndConvert(videoContainer,sourceFile);err!=nil {
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
					fmt.Printf("\rEncode %s Speed: %.1fx Remaining: %s EstimatedAt: %02d:%02d", line, speed, durafmt.Parse(eta).LimitFirstN(2).String(), estimatedTime.Hour(), estimatedTime.Minute())
				}

				if FFMPEGProgress-lastProgressEvent > 5 {
					eta := time.Duration(float64(100*(now.Unix()-startTime.Unix()))/FFMPEGProgress) * time.Second
					J.sendEvent(model.FFMPEGSNotification, model.StartedNotificationStatus, fmt.Sprintf("{\"progress\":%.2f,\"remaining\":\"%s\"}", FFMPEGProgress, durafmt.Parse(eta).LimitFirstN(2).String()))
					lastProgressEvent = FFMPEGProgress
				}
			}
		}
		fmt.Printf("\n")
	}()
	encodedFilePath, err := J.FFMPEG(sourceFile, videoContainer, FFMPEGProgressChan)
	if err != nil {
		log.Error(err)
		//<-time.After(time.Minute*30)
		J.sendEvent(model.FFMPEGSNotification, model.FailedNotificationStatus, err.Error())
		return err
	}
	fmt.Println(encodedFilePath)

	J.sendEvent(model.UploadNotification, model.StartedNotificationStatus, "")
	err = J.UploadJob(err, encodedFilePath, sourceFile)
	if err != nil {
		J.sendEvent(model.UploadNotification, model.FailedNotificationStatus, "")
		return err
	}
	J.sendEvent(model.UploadNotification, model.CompletedNotificationStatus, "")
	return nil
}
func (J *EncodeWorker) GetTaskID() uuid.UUID {
	return J.task.Id
}
func (J *EncodeWorker) Cancel() {
	log.Warnf("[%s] Canceling job %s...", J.GetID(), J.task.Id.String())
	J.cancelContext()
	//TODO hauriem de esperar de alguna manera a asegurar que el Execute() ha sortit..
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

func (J *EncodeWorker) PGSMkvExtractDetectAndConvert(container *ContainerData,sourceFile string) error {
	var PGSTOSrt []*Subtitle
	for _,subt := range container.Subtitle {
		if subt.isImageTypeSubtitle() {
			PGSTOSrt=append(PGSTOSrt,subt)
		}
	}
	if len(PGSTOSrt) > 0 {
		J.sendEvent(model.MKVExtractNotification,model.StartedNotificationStatus,"")
		err := J.MKVExtract(PGSTOSrt,sourceFile)
		if err!=nil{
			J.sendEvent(model.MKVExtractNotification,model.FailedNotificationStatus,err.Error())
			return err
		}else{
			J.sendEvent(model.MKVExtractNotification,model.CompletedNotificationStatus,"")
		}

		J.sendEvent(model.PGSNotification,model.StartedNotificationStatus,"")
		err = J.convertPGSToSrt(container,PGSTOSrt)
		if err!=nil {
			J.sendEvent(model.PGSNotification,model.FailedNotificationStatus,err.Error())
			return err
		}else{
			J.sendEvent(model.PGSNotification,model.CompletedNotificationStatus,"")
		}
	}
	return nil
}

func (J *EncodeWorker) convertPGSToSrt(container *ContainerData,subtitles []*Subtitle) error {
	out := make(chan *model.TaskPGSResponse)
	var pendingPGSResponses []<-chan *model.TaskPGSResponse
	for _,subtitle:=range subtitles{
		subFile, err := os.Open(filepath.Join(J.tempPath,fmt.Sprintf("%d.sup",subtitle.Id)))
		if err!=nil {
			return err
		}
		outputBytes,err := ioutil.ReadAll(subFile)
		if err!=nil {
			return err
		}
		subFile.Close()
		log.Infof("Subtitle %d is PGS, requesting  conversion...", subtitle.Id)
		PGSResponse := J.RequestPGSJob(model.TaskPGS{
			Id:          J.GetTaskID(),
			PGSID:       int(subtitle.Id),
			PGSdata:     outputBytes,
			PGSLanguage: subtitle.Language,
		})
		pendingPGSResponses =append(pendingPGSResponses,PGSResponse)
	}
	go func() {
		for _, c := range pendingPGSResponses {
			for v := range c {
				out <- v
			}
		}
		close(out)
	}()

	for {
		select {
		case <-J.ctx.Done():
			return J.ctx.Err()
		case <-time.After(time.Minute * 15):
			return errors.New("timeout Waiting for PGS Job Done")
		case response, ok := <-out:
			if !ok {
				return nil
			}
			if response.Err != "" {
				return fmt.Errorf("error on Process PGS %d: %s",response.PGSID,response.Err)
			}
			subtFilePath:=filepath.Join(J.tempPath,fmt.Sprintf("%d.srt",response.PGSID))
			err := ioutil.WriteFile(subtFilePath,response.Srt,os.ModePerm)
			if err!=nil {
				return err
			}
		}
	}
}

func (J *EncodeWorker) MKVExtract(subtitles []*Subtitle,sourceFile string) error {
	mkvExtractCommand:=command.NewCommand(helper.GetMKVExtractPath(),"tracks",sourceFile).
		SetWorkDir(J.tempPath)
		if runtime.GOOS =="linux" {
			mkvExtractCommand.AddEnv(fmt.Sprintf("LD_LIBRARY_PATH=%s",filepath.Dir(helper.GetMKVExtractPath())))
		}
	for _,subtitle := range subtitles {
		mkvExtractCommand.AddParam(fmt.Sprintf("%d:%d.sup",subtitle.Id,subtitle.Id))
	}

	_,err := mkvExtractCommand.RunWithContext(J.ctx,command.NewAllowedCodesOption(0,1))
	if err!=nil{
		log.Debugf("MKVExtract Command:%s",mkvExtractCommand.GetFullCommand())
		return fmt.Errorf("MKVExtract unexpected error:%v",err.Error())
		return err
	}

	return nil
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
		codecQuality := fmt.Sprintf("-c:a:%d %s -vbr %d", index,"aac", 5)
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
	subtInputIndex := 1
	for index, subtitle := range container.Subtitle {
		if subtitle.isImageTypeSubtitle() {
			subtitleMap := fmt.Sprintf("-map %d -c:s:%d srt", subtInputIndex,index)
			subtitleForced := ""
			subtitleComment := ""
			if subtitle.Forced {
				subtitleForced = fmt.Sprintf(" -disposition:s:s:%d forced  -disposition:s:s:%d default", index, index)
			}
			if subtitle.Comment {
				subtitleComment = fmt.Sprintf(" -disposition:s:s:%d comment", index)
			}

			title:=""
			if subtitle.Title !=""{
				title=fmt.Sprintf("\"%s\"",subtitle.Title)
			}
			F.SubtitleFilter = append(F.SubtitleFilter, fmt.Sprintf("%s %s %s -metadata:s:s:%d language=%s -metadata:s:s:%d title=%s", subtitleMap, subtitleForced,subtitleComment, index, subtitle.Language, index, title))
			subtInputIndex++
		} else {
			F.SubtitleFilter = append(F.SubtitleFilter, fmt.Sprintf("-map 0:%d -c:s:%d copy -sub_charenc UTF-8", subtitle.Id,index))
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
		inputsParameters = fmt.Sprintf("%s  -i \"%s\"", inputsParameters, input)
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

func (F *FFMPEGGenerator) setInputFilters(container *ContainerData,sourceFilePath string,tempPath string) {
	F.inputPaths = append(F.inputPaths, sourceFilePath)
	inputIndex:=0
	if container.HaveImageTypeSubtitle() {
		for _,subt := range container.Subtitle {
			if subt.isImageTypeSubtitle(){
				inputIndex++
				F.inputPaths = append(F.inputPaths, filepath.Join(tempPath,fmt.Sprintf("%d.srt",subt.Id)))
			}
		}
	}
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
	return strings.Index(strings.ToLower(C.Format), "pgs") != -1
}

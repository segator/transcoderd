package ffmpeg

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"gopkg.in/vansante/go-ffprobe.v2"
	"os"
	"strconv"
	"strings"
	"time"
)

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
type NormalizedFFProbe struct {
	Video    *Video
	Audios   []*Audio
	Subtitle []*Subtitle
}

func (c *NormalizedFFProbe) HaveImageTypeSubtitle() bool {
	for _, sub := range c.Subtitle {
		if sub.IsImageTypeSubtitle() {
			return true
		}
	}
	return false
}

func (c *NormalizedFFProbe) GetPGSSubtitles() []*Subtitle {
	var PGSTOSrt []*Subtitle
	for _, subt := range c.Subtitle {
		if subt.IsImageTypeSubtitle() {
			PGSTOSrt = append(PGSTOSrt, subt)
		}
	}
	return PGSTOSrt
}

func (c *NormalizedFFProbe) ToJson() string {
	b, err := json.Marshal(c)
	if err != nil {
		panic(err)
	}
	return string(b)
}
func (s *Subtitle) IsImageTypeSubtitle() bool {
	return strings.Contains(strings.ToLower(s.Format), "pgs")
}

func ExtractFFProbeData(ctx context.Context, inputFile string) (data *ffprobe.ProbeData, err error) {
	fileReader, err := os.Open(inputFile)
	if err != nil {
		return nil, fmt.Errorf("error opening file %s because %v", inputFile, err)
	}

	defer fileReader.Close()
	data, err = ffprobe.ProbeReader(ctx, fileReader)
	if err != nil {
		return nil, fmt.Errorf("error getting data: %v", err)
	}
	return data, nil
}

func ffProbeFrameRate(ffprobeFrameRate string) (frameRate int, err error) {
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

func NormalizeFFProbeData(data *ffprobe.ProbeData) (container *NormalizedFFProbe, err error) {
	container = &NormalizedFFProbe{}

	videoStream := data.StreamType(ffprobe.StreamVideo)[0]
	frameRate, err := ffProbeFrameRate(videoStream.AvgFrameRate)
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

package ffmpeg

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"gopkg.in/vansante/go-ffprobe.v2"
	"os/exec"
	"regexp"
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

// GetExtractableSubtitles returns subtitle streams that need mkvextract + conversion to SRT.
// These are streams with codecs that FFmpeg cannot read from MKV (e.g. S_TEXT/WEBVTT).
func (c *NormalizedFFProbe) GetExtractableSubtitles() []*Subtitle {
	var extractable []*Subtitle
	for _, subt := range c.Subtitle {
		if subt.NeedsMKVExtraction() {
			extractable = append(extractable, subt)
		}
	}
	return extractable
}

// HaveExtractableSubtitle returns true if the container has any subtitle that needs
// mkvextract (either PGS for OCR or unsupported codecs for conversion).
func (c *NormalizedFFProbe) HaveExtractableSubtitle() bool {
	for _, sub := range c.Subtitle {
		if sub.IsImageTypeSubtitle() || sub.NeedsMKVExtraction() {
			return true
		}
	}
	return false
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

// NeedsMKVExtraction returns true if the subtitle stream has an unknown or unsupported codec
// that FFmpeg cannot read directly from MKV but can be handled by extracting with mkvextract
// and converting to SRT.
// This happens when MKV files contain subtitle tracks with codec IDs that FFmpeg doesn't
// recognize (e.g. S_TEXT/WEBVTT from mkvmerge v85+), causing ffprobe to report the codec
// as empty/"none". It also catches codecs incompatible with MKV output (e.g. mov_text).
// These streams are extracted via mkvextract, converted to SRT, and fed back to FFmpeg.
func (s *Subtitle) NeedsMKVExtraction() bool {
	if s.IsImageTypeSubtitle() {
		return false
	}
	format := strings.ToLower(s.Format)
	if format == "" || format == "none" {
		return true
	}
	if format == "mov_text" {
		return true
	}
	return false
}

func ExtractFFProbeData(ctx context.Context, inputFile string) (data *ffprobe.ProbeData, err error) {
	args := []string{
		"-loglevel", "fatal",
		"-print_format", "json",
		"-show_format",
		"-show_streams",
		"-show_chapters",
		inputFile,
	}

	cmd := exec.CommandContext(ctx, "ffprobe", args...)
	var outputBuf, stdErrBuf bytes.Buffer
	cmd.Stdout = &outputBuf
	cmd.Stderr = &stdErrBuf

	if err := cmd.Run(); err != nil {
		return nil, fmt.Errorf("error running ffprobe on %s [%s]: %w", inputFile, stdErrBuf.String(), err)
	}

	jsonBytes := sanitizeFFProbeJSON(outputBuf.Bytes())

	data = &ffprobe.ProbeData{}
	if err := json.Unmarshal(jsonBytes, data); err != nil {
		return nil, fmt.Errorf("error parsing ffprobe output: %w", err)
	}
	return data, nil
}

var invertedFieldRegex = regexp.MustCompile(`"inverted"\s*:\s*(\d+)`)

func sanitizeFFProbeJSON(b []byte) []byte {
	return invertedFieldRegex.ReplaceAllFunc(b, func(match []byte) []byte {
		submatch := invertedFieldRegex.FindSubmatch(match)
		if len(submatch) < 2 {
			return match
		}
		if string(submatch[1]) == "0" {
			return []byte(`"inverted": false`)
		}
		return []byte(`"inverted": true`)
	})
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
		language, _ := stream.TagList.GetString("language")
		title, _ := stream.TagList.GetString("title")
		newAudio := &Audio{
			Id:             uint8(stream.Index),
			Language:       language,
			Channels:       stream.ChannelLayout,
			ChannelsNumber: uint8(stream.Channels),
			ChannelLayour:  stream.ChannelLayout,
			Default:        stream.Disposition.Default == 1,
			Bitrate:        uint(bitRateInt),
			Title:          title,
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
			betterAudioStreamPerLanguage[newAudio.Language] = newAudio
		}

	}
	for _, audioStream := range betterAudioStreamPerLanguage {
		container.Audios = append(container.Audios, audioStream)
	}

	betterSubtitleStreamPerLanguage := make(map[string]*Subtitle)
	for _, stream := range data.StreamType(ffprobe.StreamSubtitle) {
		language, _ := stream.TagList.GetString("language")
		title, _ := stream.TagList.GetString("title")
		newSubtitle := &Subtitle{
			Id:       uint8(stream.Index),
			Language: language,
			Forced:   stream.Disposition.Forced == 1,
			Comment:  stream.Disposition.Comment == 1,
			Format:   stream.CodecName,
			Title:    title,
		}

		if newSubtitle.Forced || newSubtitle.Comment {
			container.Subtitle = append(container.Subtitle, newSubtitle)
			continue
		}
		// TODO Filter Languages we don't want
		betterSubtitle := betterSubtitleStreamPerLanguage[newSubtitle.Language]
		if betterSubtitle == nil { // TODO Potser perdem subtituls que es necesiten
			betterSubtitleStreamPerLanguage[newSubtitle.Language] = newSubtitle
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

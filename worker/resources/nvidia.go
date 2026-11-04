package resources

import (
	"bytes"
	"context"
	"encoding/xml"
	"errors"
	"io"
	"math"
	"os/exec"
	"strconv"
	"strings"
	"time"

	"transcoder/model"
)

const gpuQueryTimeout = 2 * time.Second
const maxGPUQueryBytes = 2 * 1024 * 1024

type powerReadings struct {
	Draw    string `xml:"power_draw"`
	Instant string `xml:"instant_power_draw"`
	Average string `xml:"average_power_draw"`
	Limit   string `xml:"power_limit"`
	Current string `xml:"current_power_limit"`
}

type nvidiaGPU struct {
	BusID string `xml:"id,attr"`
	UUID  string `xml:"uuid"`
	Name  string `xml:"product_name"`
	Usage struct {
		GPU     string `xml:"gpu_util"`
		Encoder string `xml:"encoder_util"`
		Decoder string `xml:"decoder_util"`
	} `xml:"utilization"`
	Memory struct {
		Used  string `xml:"used"`
		Total string `xml:"total"`
	} `xml:"fb_memory_usage"`
	Temperature struct {
		GPU string `xml:"gpu_temp"`
	} `xml:"temperature"`
	Power       powerReadings `xml:"gpu_power_readings"`
	LegacyPower powerReadings `xml:"power_readings"`
}

func metric(raw, unit string) *float64 {
	parts := strings.Fields(raw)
	if len(parts) != 2 || parts[1] != unit {
		return nil
	}
	n, err := strconv.ParseFloat(parts[0], 64)
	if err != nil || math.IsNaN(n) || math.IsInf(n, 0) || n < 0 || (unit == "%" && n > 100) {
		return nil
	}
	return &n
}

func memoryBytes(raw string) *uint64 {
	n := metric(raw, "MiB")
	if n == nil || *n >= float64(math.MaxUint64)/1048576 {
		return nil
	}
	value := uint64(*n * 1048576)
	return &value
}

func firstMetric(unit string, values ...string) *float64 {
	for _, raw := range values {
		if n := metric(raw, unit); n != nil {
			return n
		}
	}
	return nil
}

func parseGPUs(raw []byte) ([]model.GPUResources, string, error) {
	var report struct {
		XMLName xml.Name    `xml:"nvidia_smi_log"`
		Driver  string      `xml:"driver_version"`
		KMD     string      `xml:"kmd_version"`
		GPUs    []nvidiaGPU `xml:"gpu"`
	}
	if len(raw) > maxGPUQueryBytes {
		return nil, "", errors.New("GPU query output exceeds limit")
	}
	if err := xml.Unmarshal(raw, &report); err != nil {
		return nil, "", err
	}
	driver := report.KMD
	if driver == "" {
		driver = report.Driver
	}
	gpus := make([]model.GPUResources, 0, len(report.GPUs))
	for _, g := range report.GPUs {
		gpu := model.GPUResources{UUID: g.UUID, Name: g.Name, BusID: g.BusID,
			Percent: metric(g.Usage.GPU, "%"), EncoderPercent: metric(g.Usage.Encoder, "%"), DecoderPercent: metric(g.Usage.Decoder, "%"),
			MemoryUsedBytes: memoryBytes(g.Memory.Used), MemoryTotalBytes: memoryBytes(g.Memory.Total),
			TemperatureC:    metric(g.Temperature.GPU, "C"),
			PowerWatts:      firstMetric("W", g.Power.Instant, g.Power.Draw, g.Power.Average, g.LegacyPower.Draw, g.LegacyPower.Instant, g.LegacyPower.Average),
			PowerLimitWatts: firstMetric("W", g.Power.Current, g.Power.Limit, g.LegacyPower.Current, g.LegacyPower.Limit)}
		if gpu.MemoryUsedBytes != nil && gpu.MemoryTotalBytes != nil && *gpu.MemoryUsedBytes > *gpu.MemoryTotalBytes {
			gpu.MemoryUsedBytes, gpu.MemoryTotalBytes = nil, nil
		}
		gpus = append(gpus, gpu)
	}
	return gpus, driver, nil
}

type boundedOutput struct{ buffer bytes.Buffer }

func (b *boundedOutput) Write(p []byte) (int, error) {
	if b.buffer.Len()+len(p) > maxGPUQueryBytes {
		return 0, errors.New("GPU query output exceeds limit")
	}
	return b.buffer.Write(p)
}

func queryGPU(ctx context.Context, executable string) ([]model.GPUResources, string, string) {
	ctx, cancel := context.WithTimeout(ctx, gpuQueryTimeout)
	defer cancel()
	cmd := exec.CommandContext(ctx, executable, "-q", "-x")
	cmd.WaitDelay = time.Second
	var output boundedOutput
	cmd.Stdout, cmd.Stderr = &output, io.Discard
	if err := cmd.Run(); err != nil {
		return []model.GPUResources{}, "", "unavailable"
	}
	gpus, driver, err := parseGPUs(output.buffer.Bytes())
	if err != nil {
		return []model.GPUResources{}, "", "unavailable"
	}
	state := "available"
	if len(gpus) == 0 {
		state = "no-devices"
	}
	return gpus, driver, state
}

func readGPUs(ctx context.Context) ([]model.GPUResources, string, string) {
	executable, err := exec.LookPath("nvidia-smi")
	if err != nil {
		return []model.GPUResources{}, "", "unavailable"
	}
	return queryGPU(ctx, executable)
}

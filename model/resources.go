package model

import "time"

type WorkerResources struct {
	SampledAt     time.Time        `json:"sampledAt"`
	Hostname      string           `json:"hostname"`
	OS            string           `json:"os"`
	Architecture  string           `json:"architecture"`
	CPU           *CPUResources    `json:"cpu"`
	Memory        *MemoryResources `json:"memory"`
	GPUs          []GPUResources   `json:"gpus"`
	GPUStatus     string           `json:"gpuStatus"`
	DriverVersion string           `json:"driverVersion,omitempty"`
}

type CPUResources struct {
	Percent *float64 `json:"percent"`
	Cores   int      `json:"cores"`
	Scope   string   `json:"scope"`
}

type MemoryResources struct {
	UsedBytes  uint64 `json:"usedBytes"`
	TotalBytes uint64 `json:"totalBytes"`
}

type GPUResources struct {
	UUID             string   `json:"uuid"`
	Name             string   `json:"name"`
	BusID            string   `json:"busId"`
	Percent          *float64 `json:"percent"`
	EncoderPercent   *float64 `json:"encoderPercent"`
	DecoderPercent   *float64 `json:"decoderPercent"`
	MemoryUsedBytes  *uint64  `json:"memoryUsedBytes"`
	MemoryTotalBytes *uint64  `json:"memoryTotalBytes"`
	TemperatureC     *float64 `json:"temperatureC"`
	PowerWatts       *float64 `json:"powerWatts"`
	PowerLimitWatts  *float64 `json:"powerLimitWatts"`
}

package model

import "fmt"

const IndependentEncoderCapability = "independent-encoders"

// WorkerSettings is either one legacy maximum or one atomic CPU/GPU pair.
type WorkerSettings struct {
	MaxActiveJobs *int `json:"maxActiveJobs"`
	CPUJobs       *int `json:"cpuJobs,omitempty"`
	GPUJobs       *int `json:"gpuJobs,omitempty"`
}

func (s WorkerSettings) Independent() bool { return s.CPUJobs != nil && s.GPUJobs != nil }

func (s WorkerSettings) ValidateWrite() error {
	if s.CPUJobs != nil || s.GPUJobs != nil {
		if !s.Independent() || s.MaxActiveJobs != nil {
			return fmt.Errorf("supply both cpuJobs and gpuJobs without maxActiveJobs")
		}
		_, err := s.Total()
		return err
	}
	if s.MaxActiveJobs == nil || *s.MaxActiveJobs < 0 {
		return fmt.Errorf("maxActiveJobs must be a non-negative integer")
	}
	return nil
}

func (s WorkerSettings) Total() (int, error) {
	if !s.Independent() || *s.CPUJobs < 0 || *s.GPUJobs < 0 {
		return 0, fmt.Errorf("cpuJobs and gpuJobs must be non-negative integers")
	}
	max := int(^uint(0) >> 1)
	if *s.CPUJobs > max-*s.GPUJobs {
		return 0, fmt.Errorf("cpuJobs plus gpuJobs exceeds the supported integer range")
	}
	return *s.CPUJobs + *s.GPUJobs, nil
}

func (s WorkerSettings) Clone() WorkerSettings {
	clone := s
	if s.MaxActiveJobs != nil {
		v := *s.MaxActiveJobs
		clone.MaxActiveJobs = &v
	}
	if s.CPUJobs != nil {
		v := *s.CPUJobs
		clone.CPUJobs = &v
	}
	if s.GPUJobs != nil {
		v := *s.GPUJobs
		clone.GPUJobs = &v
	}
	return clone
}

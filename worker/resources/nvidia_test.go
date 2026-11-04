package resources

import "testing"

func TestNvidiaMetricsPartialAndMultipleDevices(t *testing.T) {
	raw := []byte(`<nvidia_smi_log><driver_version>old</driver_version><kmd_version>current</kmd_version><gpu id="pci-1"><uuid>gpu-a</uuid><product_name>GPU A</product_name><utilization><gpu_util>0 %</gpu_util><encoder_util>82 %</encoder_util><decoder_util>N/A</decoder_util></utilization><fb_memory_usage><used>2048 MiB</used><total>8192 MiB</total></fb_memory_usage><temperature><gpu_temp>40.50 C</gpu_temp></temperature><gpu_power_readings><instant_power_draw>105.5 W</instant_power_draw><current_power_limit>300 W</current_power_limit></gpu_power_readings></gpu><gpu><uuid>gpu-b</uuid><utilization><gpu_util>101 %</gpu_util><encoder_util>N/A</encoder_util></utilization><power_readings><power_draw>0 W</power_draw><power_limit>250 W</power_limit></power_readings></gpu></nvidia_smi_log>`)
	gpus, driver, err := parseGPUs(raw)
	if err != nil || driver != "current" || len(gpus) != 2 {
		t.Fatalf("parseGPUs() = %+v %s %v", gpus, driver, err)
	}
	a, b := gpus[0], gpus[1]
	if a.Percent == nil || *a.Percent != 0 || a.EncoderPercent == nil || *a.EncoderPercent != 82 || a.DecoderPercent != nil || *a.MemoryUsedBytes != 2048*1024*1024 || *a.MemoryTotalBytes != 8192*1024*1024 || *a.TemperatureC != 40.5 || *a.PowerWatts != 105.5 || *a.PowerLimitWatts != 300 {
		t.Errorf("device A = %+v", a)
	}
	if b.Percent != nil || b.EncoderPercent != nil || b.MemoryUsedBytes != nil || b.TemperatureC != nil || b.PowerWatts == nil || *b.PowerWatts != 0 || *b.PowerLimitWatts != 250 {
		t.Errorf("device B = %+v", b)
	}
	for _, raw := range []string{`<wrong/>`, `<nvidia_smi_log>broken`, string(make([]byte, maxGPUQueryBytes+1))} {
		if _, _, err := parseGPUs([]byte(raw)); err == nil {
			t.Error("accepted invalid or oversized XML")
		}
	}
	for _, raw := range []string{"N/A", "NaN %", "Inf %", "-1 %", "101 %", "12 W"} {
		if metric(raw, "%") != nil {
			t.Errorf("invalid percentage %q accepted", raw)
		}
	}
}

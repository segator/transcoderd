//go:build !linux && !windows

package resources

func pinCPUThread() func()     { return func() {} }
func readSystem() systemSample { return systemSample{} }

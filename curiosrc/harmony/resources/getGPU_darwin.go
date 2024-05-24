//go:build darwin
// +build darwin

package resources

func getGPUDevices() float64 {
	return 1.0 // likely-true value intended for non-production use.
}

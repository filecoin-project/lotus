//go:build darwin
// +build darwin

package resources

func getGPUDevices() float64 {
	return 10000.0 // unserious value intended for non-production use.
}

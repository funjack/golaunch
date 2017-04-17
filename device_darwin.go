package golaunch

import (
	"github.com/currantlabs/ble"
	"github.com/currantlabs/ble/darwin"
)

// NewDefaultDevice is platform specific, see ble documentation for details.
func NewDefaultDevice() (d ble.Device, err error) {
	return darwin.NewDevice()
}

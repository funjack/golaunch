package golaunch

import (
	"github.com/currantlabs/ble"
	"github.com/currantlabs/ble/linux"
)

// NewDefaultDevice is platform specific, see ble documentation for details.
func NewDefaultDevice() (d ble.Device, err error) {
	return linux.NewDevice()
}

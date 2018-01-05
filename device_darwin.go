package golaunch

import (
	"github.com/go-ble/ble"
	"github.com/go-ble/ble/darwin"
)

// NewDefaultDevice is platform specific, see ble documentation for details.
func NewDefaultDevice() (d ble.Device, err error) {
	return darwin.NewDevice()
}

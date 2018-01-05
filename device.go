// +build !linux,!darwin

package golaunch

import (
	"fmt"
	"runtime"

	"github.com/go-ble/ble"
)

// NewDefaultDevice is platform specific, see ble documentation for details.
func NewDefaultDevice() (d ble.Device, err error) {
	return nil, fmt.Errorf("ble not supported on %s", runtime.GOOS)
}

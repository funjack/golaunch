package golaunch

import "github.com/currantlabs/gatt"

// DefaultClientOptions are platform specific connection options, see gatt
// documentation for details.
var DefaultClientOptions = []gatt.Option{
	gatt.LnxMaxConnections(1),
	gatt.LnxDeviceID(-1, false),
}

package golaunch

import (
	"context"
	"errors"
	"strings"
	"time"

	"github.com/go-ble/ble"
)

var (
	// Bluetooth device name of the Launch
	name = "Launch"
	// The main service UUID for the Launch
	serviceID = ble.MustParse("88f80580-0000-01e6-aace-0002a5d5c51b")
	// Command characteristic (Write)
	cmdCharID = ble.MustParse("88f80581-0000-01e6-aace-0002a5d5c51b")
	// Touch events characteristic (Notifications)
	touchCharID = ble.MustParse("88f80582-0000-01e6-aace-0002a5d5c51b")
	// Mode? characteristic (Read/Write)
	modeCharID = ble.MustParse("88f80583-0000-01e6-aace-0002a5d5c51b")
)

var (
	// ErrDisconnected is the error returned when there is no connection.
	ErrDisconnected = errors.New("disconnected")
	// ErrDiscover is the error returned when there was an issue
	// discovering the Launch.
	ErrDiscover = errors.New("could not discover launch")
	// ErrInit is the error returned when there was an issue initializing
	// the Launch.
	ErrInit = errors.New("initialization error")
	// ErrUnknownMode is the error returned when someone wants to send a
	// (yet) unknown mode.
	ErrUnknownMode = errors.New("unknown mode")
)

var (
	// Time for the device to get ready before sending data.
	readyTime = time.Second * 2
	// Minimum amount of time between write operations.
	threshold = time.Millisecond * 100
	// Amount of write events to buffer before blocking.
	writeBufferSize = 10
)

const (
	// modeReadValuesAsBytes will cause the Launch to read position/speed
	// values as bytes instead of BCD. Also seems to fix it from having
	// spasms.
	modeReadValuesAsBytes = 0x00
)

// Launch interface represents a device that can move like a Launch.
type Launch interface {
	// Connect will (re)connect to the Launch.
	Connect(ctx context.Context) error

	// Disconnect will disconnect to the Launch.
	Disconnect()

	// Move moves to position at speed in percent.
	Move(position, speed int)

	// HandleDisconnect registers a function to call when a device disconnects
	HandleDisconnect(f func())
}

// NewLaunch creates and returns a Launch client that can be used to
// communicate.
func NewLaunch() Launch {
	l := &launch{
		disconnect: make(chan bool),
		wbuffer:    make(chan [2]byte, writeBufferSize),
		limiter:    time.Tick(threshold),
	}
	return l
}

// NewLaunchWithDevice creates and returns a Launch client that can be used to
// communicate over specified Bluetooth device.
func NewLaunchWithDevice(d ble.Device) Launch {
	l, _ := NewLaunch().(*launch)
	l.device = d
	ble.SetDefaultDevice(d)
	return l

}

// launch is the structure used to manage the connection to a Launch.
type launch struct {
	device ble.Device
	client ble.Client

	cmd  *ble.Characteristic
	mode *ble.Characteristic

	disconnect chan bool
	wbuffer    chan [2]byte
	limiter    <-chan time.Time

	disconnectFunc func()
}

// Connect initializes configured Bluetooth device and creates a connection to
// a Launch.
func (l *launch) Connect(ctx context.Context) (err error) {
	// Claim a Bluetooth device
	if l.device == nil {
		l.device, err = NewDefaultDevice()
		if err != nil {
			return err
		}
		ble.SetDefaultDevice(l.device)
	}

	// Still connected
	if l.client != nil {
		return nil
	}

	// Discover the Launch and it's characteristics
	err = l.discover(ctx)
	if err != nil {
		return err
	}

	// Give the Launch a little bit of time to get ready
	<-time.After(readyTime)

	// Put the Launch into a mode that it will actually work well
	err = l.writeMode(modeReadValuesAsBytes)
	if err != nil {
		l.client.CancelConnection()
		l.cleanupClient()
		return ErrInit
	}

	// Handle disconnects
	stopWriting := make(chan bool)
	go func() {
		select {
		case <-l.client.Disconnected():
		case <-l.disconnect:
			l.client.CancelConnection()
		}
		stopWriting <- true
		l.cleanupClient()

		if l.disconnectFunc != nil {
			l.disconnectFunc()
		}
	}()
	// Start the goroutine that will write commands
	go l.writeFromBuffer(stopWriting)

	return nil
}

// discover scans, connects and discovers characteristics of a Launch.
func (l *launch) discover(ctx context.Context) error {
	// Connect to Launch
	client, err := ble.Connect(ctx, launchAdvFilter)
	if err != nil {
		return err
	}

	// Discover Service
	s, err := client.DiscoverServices([]ble.UUID{serviceID})
	if err != nil {
		return err
	}
	if len(s) != 1 {
		return ErrDiscover
	}

	// Discover Launch characteristics
	cs, err := client.DiscoverCharacteristics(
		[]ble.UUID{cmdCharID, modeCharID}, s[0])
	if err != nil {
		return err
	}
	var cmd, mode *ble.Characteristic
	for _, c := range cs {
		switch {
		case c.UUID.Equal(cmdCharID):
			cmd = c
		case c.UUID.Equal(modeCharID):
			mode = c
		}
	}
	if cmd == nil || mode == nil {
		return ErrDiscover
	}

	// Store found device characteristics
	l.client = client
	l.cmd = cmd
	l.mode = mode

	return nil
}

// launchAdvFilter implements ble.AdvFilter and filters for a Launch.
func launchAdvFilter(a ble.Advertisement) bool {
	if strings.ToUpper(a.LocalName()) == strings.ToUpper(name) {
		return true
	}
	return false
}

// cleanupClient removes all references to a disconnected client.
func (l *launch) cleanupClient() {
	if l.client != nil {
		l.client = nil
		l.mode = nil
		l.cmd = nil
	}
}

// Disconnect cancels the connection with the Launch.
func (l *launch) Disconnect() {
	select {
	case l.disconnect <- true:
	default:
	}
}

// writeMode will send specified mode to the Launch. Only support putting
// the Launch in "read values as bytes", so it interprets values on the command
// channel as bytes instead of binary coded decimals.
func (l *launch) writeMode(c byte) error {
	if l.client == nil {
		return ErrDisconnected
	}
	if c == modeReadValuesAsBytes {
		err := l.client.WriteCharacteristic(l.mode, []byte{c}, false)
		return err
	}
	return ErrUnknownMode
}

// writeFromBuffer sends commands to the Launch that are stored in the write
// buffer. To stop this function send true on the l.stopWriting channel.
func (l *launch) writeFromBuffer(stopchan <-chan bool) {
	for {
		select {
		case stop := <-stopchan:
			if stop == true {
				return
			}
		case b := <-l.wbuffer:
			// Limit amount of writes to avoid disconnects
			<-l.limiter
			l.client.WriteCharacteristic(l.cmd, b[:], true)
		}
	}
}

// Move will move to the specified position at the desired speed.
// Position and speed are specified in percent.
func (l *launch) Move(position, speed int) {
	// Make sure we remain in the limits
	if position < 0 {
		position = 0
	} else if position > 99 {
		position = 99
	}

	// Don't go below 20% as very slow speeds seem to crash the Launch
	if speed < 20 {
		speed = 20
	} else if speed > 99 {
		speed = 99
	}

	data := [2]byte{byte(position), byte(speed)}
	l.wbuffer <- data
}

// HandleDisconnect registers a function that is called when the Launch
// disconnects.
func (l *launch) HandleDisconnect(f func()) {
	l.disconnectFunc = f
}

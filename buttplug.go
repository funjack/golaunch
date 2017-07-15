package golaunch

import (
	"context"
	"crypto/tls"
	"time"

	"github.com/funjack/golibbuttplug"
)

const buttplugLaunchName = "Fleshlight Launch"

// buttplugLauch is a Launch connected via Buttplug.
type buttplugLaunch struct {
	ctx    context.Context
	addr   string
	tlscfg *tls.Config
	name   string

	client *golibbuttplug.Client
	device *golibbuttplug.Device

	disconnect chan bool
	wbuffer    chan [2]int
	limiter    <-chan time.Time

	disconnectFunc func()
}

// NewButtplugLaunch creates a new Launch connected via the Buttplug server
// running at addr. Identify with the Buttplug server with the given name.
func NewButtplugLaunch(ctx context.Context, addr, name string, tlscfg *tls.Config) Launch {
	return &buttplugLaunch{
		ctx:        ctx,
		addr:       addr,
		tlscfg:     tlscfg,
		name:       name,
		disconnect: make(chan bool),
		wbuffer:    make(chan [2]int, writeBufferSize),
		limiter:    time.Tick(threshold),
	}
}

// connect to Buttplug.
func (l *buttplugLaunch) connect() error {
	if l.client != nil {
		select {
		case <-l.client.Disconnected():
		default:
			// Still connected to buttplug.
			return nil
		}
	}
	c, err := golibbuttplug.NewClient(l.ctx, l.addr, l.name, l.tlscfg)
	if err != nil {
		return err
	}
	l.client = c
	return nil
}

// Connect sets up a connection with Buttplug and creates a connection with
// a Launch.
func (l *buttplugLaunch) Connect(ctx context.Context) error {
	// Connect to Buttplug
	if err := l.connect(); err != nil {
		return err
	}
	// Still have a device connected
	if l.device != nil {
		select {
		case <-l.device.Disconnected():
		default:
			return nil
		}
	}
	if err := l.client.StartScanning(); err != nil {
		return err
	}
	// Wait for scanning to finish.
	scanctx, cancel := context.WithTimeout(ctx, 30*time.Second)
	err := l.client.WaitOnScanning(scanctx)
	cancel()
	if err == context.DeadlineExceeded {
		// Stop scanning.
		if err := l.client.StopScanning(); err != nil {
			return err
		}
	} else if err != nil {
		return err
	}
	// Get all known devices.
	for _, d := range l.client.Devices() {
		if d.Name() == buttplugLaunchName && d.IsSupported(golibbuttplug.CommandFleshlightLaunchFW12) {
			l.device = d
			break
		}
	}
	if l.device == nil {
		return ErrDiscover
	}
	// Handle disconnects
	stopWriting := make(chan bool)
	go func() {
		select {
		case <-l.device.Disconnected():
		case <-l.client.Disconnected():
			l.client = nil
		case <-l.disconnect:
			l.device.StopDeviceCmd()
			l.client.Close()
			l.client = nil
		}
		stopWriting <- true
		l.device = nil
		if l.disconnectFunc != nil {
			l.disconnectFunc()
		}
	}()
	go l.writeFromBuffer(stopWriting)
	return nil
}

// Disconnect cancels the connection with the Launch and Buttplug.
func (l *buttplugLaunch) Disconnect() {
	select {
	case l.disconnect <- true:
	default:
	}
}

// writeFromBuffer sends commands to the Launch that are stored in the write
// buffer. To stop this function send true on the l.stopWriting channel.
func (l *buttplugLaunch) writeFromBuffer(stopchan <-chan bool) {
	for {
		select {
		case stop := <-stopchan:
			if stop == true {
				return
			}
		case b := <-l.wbuffer:
			// Limit amount of writes to sync behavior with our
			// BLE implementation.
			<-l.limiter
			l.device.FleshlightLaunchFW12Cmd(b[0], b[1])
		}
	}
}

// Move will move to the specified position at the desired speed.
// Position and speed are specified in percent.
func (l *buttplugLaunch) Move(position, speed int) {
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

	data := [2]int{position, speed}
	l.wbuffer <- data
}

// HandleDisconnect registers a function that is called when the Launch
// disconnects.
func (l *buttplugLaunch) HandleDisconnect(f func()) {
	l.disconnectFunc = f
}

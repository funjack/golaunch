package golaunch

import (
	"errors"
	"sync"
	"time"

	"log"

	"github.com/currantlabs/gatt"
)

var (
	// Bluetooth device name of the Launch
	pName = "Launch"
	// The main service UUID for the Launch
	serviceID = gatt.MustParseUUID("88f80580-0000-01e6-aace-0002a5d5c51b")
	// Command characteristic (Write)
	cmdCharID = gatt.MustParseUUID("88f80581-0000-01e6-aace-0002a5d5c51b")
	// Touch events characteristic (Notifications)
	touchCharID = gatt.MustParseUUID("88f80582-0000-01e6-aace-0002a5d5c51b")
	// Mode? characteristic (Read/Write)
	modeCharID = gatt.MustParseUUID("88f80583-0000-01e6-aace-0002a5d5c51b")
)

var (
	// ErrConnectionTimeout is the error returned when a connection could
	// not be made in time.
	ErrConnectionTimeout = errors.New("connection timeout")
	// ErrWriteTimeout is the error returned when a write with response did
	// not complete in time.
	ErrWriteTimeout = errors.New("write timeout")
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
	// Connection timeout.
	connectionTimeout = time.Second * 10
	// Write timeout on with response writes.
	writeTimeout = time.Second * 1
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
	Connect() error

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
		mutex:          new(sync.Mutex),
		cmdDiscovered:  make(chan bool, 1),
		modeDiscovered: make(chan bool, 1),
		stopWriting:    make(chan bool, 1),
		wbuffer:        make(chan [2]byte, writeBufferSize),
		limiter:        time.Tick(threshold),
	}
	return l
}

// launch is the structure used to manage the connection to a Launch.
type launch struct {
	mutex *sync.Mutex
	p     gatt.Peripheral

	cmdDiscovered  chan bool
	cmd            *gatt.Characteristic
	modeDiscovered chan bool
	mode           *gatt.Characteristic

	stopWriting chan bool
	wbuffer     chan [2]byte
	limiter     <-chan time.Time

	disconnectFunc func()
}

// Connect initializes configured Bluetooth device and creates a connection to
// a Launch.
func (l *launch) Connect() error {
	// Open Bluetooth device
	l.mutex.Lock()
	d, err := gatt.NewDevice(DefaultClientOptions...)
	if err != nil {
		l.mutex.Unlock()
		return err
	}

	setupComplete := make(chan bool, 1)
	go func() {

		// Register Bluetooth event handlers
		d.Handle(
			gatt.PeripheralDiscovered(l.onPeripheralDiscovered),
			gatt.PeripheralConnected(l.onConnected),
			gatt.PeripheralDisconnected(l.onDisconnected),
		)
		// Start
		d.Init(onStateChanged)

		// Make sure all characteristics are discovered before starting
		for i := 0; i < 2; i++ {
			select {
			case <-l.cmdDiscovered:
			case <-l.modeDiscovered:
			}
		}
		setupComplete <- true
	}()

	select {
	case <-setupComplete:
		l.mutex.Unlock()
		// Give the Launch a little bit of time to get ready
		<-time.After(readyTime)

		// Put the Launch into a mode that it will actually work well
		err := l.writeMode(modeReadValuesAsBytes)
		if err != nil {
			return ErrInit
		}

		// Start the goroutine that will write commands
		go l.writeFromBuffer()

		return nil
	case <-time.After(connectionTimeout):
		l.mutex.Unlock()
		return ErrConnectionTimeout
	}
}

// Disconnect cancels the connection with the Launch.
func (l *launch) Disconnect() {
	if l.p != nil {
		if d := l.p.Device(); d != nil {
			d.CancelConnection(l.p)
		}
	}
}

// writeMode will send specified mode to the Launch. Only support putting
// the Launch in "read values as bytes", so it interprets values on the command
// channel as bytes instead of binary coded decimals.
func (l *launch) writeMode(c byte) error {
	errChan := make(chan error, 1)
	go func() {
		if c == modeReadValuesAsBytes {
			l.mutex.Lock()
			err := l.p.WriteCharacteristic(l.mode, []byte{c}, false)
			l.mutex.Unlock()
			errChan <- err
		}
		errChan <- nil
	}()
	select {
	case err := <-errChan:
		return err
	case <-time.After(connectionTimeout):
		return ErrWriteTimeout
	}
}

// writeFromBuffer sends commands to the Launch that are stored in the write
// buffer. To stop this function send true on the l.stopWriting channel.
func (l *launch) writeFromBuffer() {
	for {
		select {
		case stop := <-l.stopWriting:
			if stop == true {
				return
			}
		case b := <-l.wbuffer:
			// Limit amount of writes to avoid disconnects
			<-l.limiter
			l.mutex.Lock()
			l.p.WriteCharacteristic(l.cmd, b[:], true)
			l.mutex.Unlock()
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

// discoverCharacteristics finds and stores the Launch characteristics for the
// given service.
func (l *launch) discoverCharacteristics(service *gatt.Service) error {
	cs, err := l.p.DiscoverCharacteristics([]gatt.UUID{
		modeCharID,
		cmdCharID,
	}, service)
	if err != nil {
		return err
	}

	for _, c := range cs {
		if c.UUID().Equal(cmdCharID) {
			log.Println("Launch command characteristic found")

			// Store command characteristic
			l.cmd = c
			l.modeDiscovered <- true
		} else if c.UUID().Equal(modeCharID) {
			log.Println("Launch mode characteristic found")

			// Store mode characteristic
			l.mode = c
			l.cmdDiscovered <- true
		}
	}
	return nil
}

// onConnect stores the connected peripheral in *launch state and discovers and
// stores characteristics.
func (l *launch) onConnected(p gatt.Peripheral, err error) {
	log.Printf("Launch connected")
	// Store peripheral
	l.p = p

	services, err := p.DiscoverServices([]gatt.UUID{serviceID})
	if err != nil {
		return
	}

	for _, service := range services {
		if service.UUID().Equal(serviceID) {
			log.Println("Launch service found")
			l.discoverCharacteristics(service)
		}
	}
}

// HandleDisconnect registers a function that is called when the Launch
// disconnects.
func (l *launch) HandleDisconnect(f func()) {
	l.disconnectFunc = f
}

// onDisconnected will clean up stop scanning and connect if it's a Launch.
func (l *launch) onDisconnected(p gatt.Peripheral, err error) {
	log.Println("Launch disconnected")

	select {
	case l.stopWriting <- true:
	default:
	}

	p.Device().Stop()
	<-time.After(time.Second * 2)

	if l.disconnectFunc != nil {
		l.disconnectFunc()
	}
}

// onPeripheralDiscovered will stop scanning and connect if it's a Launch.
func (l *launch) onPeripheralDiscovered(p gatt.Peripheral, a *gatt.Advertisement, rssi int) {
	if a.LocalName == pName {
		// Store peripheral
		l.p = p
		log.Printf("Launch discovered")
		p.Device().StopScanning()
		p.Device().Connect(p)
	}
}

// onStateChange will start scanning for the Launch on power on.
func onStateChanged(d gatt.Device, s gatt.State) {
	switch s {
	case gatt.StatePoweredOn:
		// Scan for Launch
		d.Scan([]gatt.UUID{serviceID}, false)
		return
	default:
		d.StopScanning()
	}
}

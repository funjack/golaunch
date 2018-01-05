# Golaunch

[![GoDoc](https://godoc.org/github.com/funjack/golaunch?status.svg)](https://godoc.org/github.com/funjack/golaunch)
[![Go Report Card](https://goreportcard.com/badge/github.com/funjack/golaunch)](https://goreportcard.com/report/github.com/funjack/golaunch)

Golaunch is a BLE central for controlling a Launch. It's build using
currantlabs(/paypal's) gatt implementation.

## Setup

See the [gatt docs](https://godoc.org/github.com/go-ble/gatt#hdr-SETUP)
for the Bluetooth requirements/setup.

## Examples

### Build and run example (Linux)

```sh
go build examples/stroke/stroke.go
sudo setcap 'cap_net_raw,cap_net_admin=eip' ./stroke
./stroke
```

Golaunch is released under a [BSD-style license](./LICENSE).

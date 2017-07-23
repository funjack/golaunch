package main

import (
	"context"
	"fmt"
	"log"
	"math/rand"
	"os"
	"time"

	"github.com/funjack/golaunch"
)

func main() {

	// Non-interactive random stroke example.

	// TODO: keyboard controls for (p)ause, (k)eep doing that, and (o)max speed/range

	launchContext := context.Background()
	rand.Seed(time.Now().UTC().UnixNano())

	l := golaunch.NewLaunch()
	log.Printf("Connecting...")
	l.HandleDisconnect(func() {
		os.Exit(0)
	})
	ctx, cancel := context.WithTimeout(launchContext, time.Second*30)
	err := l.Connect(ctx)
	cancel()
	if err != nil {
		log.Fatal(err)
	}
	log.Printf("Connected to Launch")

	for {
		var count = rand.Intn(120)
		var start = rand.Intn(30)
		var end = rand.Intn(95)   // stay within official app stroke range
		var speed = rand.Intn(80) // ditto for maximum speed
		var interval = rand.Intn(350)

		// you can't feel much in <150ms so increase if necessary
		if interval < 150 {
			interval = 150
		}
		ticker := time.Tick(time.Duration(interval) * time.Millisecond)

		/* Stay within official app speed ranges. Reverse engineering efforts
		show that going too slow can crash the Launch and may have other
		as yet undiscovered consequences that could damage your device.
		Change at your own risk. */
		if speed < 20 {
			speed = 20
		}

		// if speed and interval are low then enforce longer interval
		if speed <= 30 && interval < 175 {
			interval = interval * 4
		}

		if end < start {
			end = start
		}

		// make stroke range >=40
		var posdiff = end - start
		if posdiff < 40 {
			for (end - start) < 40 {
				end++
				start--
			}
			if start < 5 {
				/* lib already handles this. make display pretty
				   and stay within official app range limits. */
				start = 5
			}
			if end > 95 {
				/* lib already handles this. make display pretty
				   and stay within official app range limits. */
				end = 95
			}
		}
		posdiff = end - start

		fmt.Printf("%d strokes / range %d (%d:%d) / speed %d / interval %dms\n",
			count, posdiff, start, end, speed, interval)
		var p int

		for j := 1; j <= count; j++ {
			fmt.Printf("%d ", j)
			if p == end {
				p = start
			} else {
				p = end
			}
			<-ticker
			l.Move(p, speed)
		}
		fmt.Printf("\n\n")
	}
}

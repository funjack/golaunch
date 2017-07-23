package main

import (
        "context"
        "flag"
        "fmt"
        "log"
        "os"
	"math/rand"
        "time"

        "github.com/funjack/golaunch"
)

var (
        start    = flag.Int("start", 5, "start position")
        end      = flag.Int("end", 95, "end position")
        speed    = flag.Int("speed", 30, "speed")
        interval = flag.Int("interval", 300, "interval in milliseconds")
)

func main() {
	// TODO: keyboard controls for (p)ause, (k)eep doing that, and (o)max speed/range

	flag.Parse()

        launchContext := context.Background()

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

        ticker := time.Tick(time.Duration(*interval) * time.Millisecond)

        go func() {

	}()

	for {
		rand.Seed(time.Now().UTC().UnixNano())
		var count = rand.Intn(120)
		var start = rand.Intn(30)
		var end = rand.Intn(100)
		var speed = rand.Intn(100)
		var interval = rand.Intn(350)

		// you can't feel much in <150ms so increase if necessary
		if interval < 150 {
			interval = 150
		}
		ticker = time.Tick(time.Duration(interval) * time.Millisecond)

		// the slow edging crowd should comment this out
		if speed < 20 {
			speed = 20
		}

		// if speed and interval are low then enforce longer interval
		if speed <= 30 && interval < 175 {
			interval = interval*4
		}

		for end < start {
			end++
		}

		// make stroke range >=40
		var posdiff = end - start
		if posdiff < 40 {
			for (end - start) < 40 {
				end++
				start--
			}
			if start < 1 {
				// lib already handles this. make display pretty
				start = 0
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

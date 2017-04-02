package main

import (
	"bufio"
	"flag"
	"fmt"
	"log"
	"os"
	"strconv"
	"time"

	"github.com/funjack/golaunch"
)

var (
	start    = flag.Int("start", 5, "start position")
	end      = flag.Int("end", 95, "end position")
	speed    = flag.Int("speed", 30, "speed")
	interval = flag.Int("interval", 1000, "interval in milliseconds")
)

func main() {
	flag.Parse()

	l := golaunch.NewLaunch()
	l.HandleDisconnect(func() {
		os.Exit(0)
	})
	err := l.Connect()
	if err != nil {
		log.Fatal(err)
	}

	ticker := time.Tick(time.Duration(*interval) * time.Millisecond)
	go func() {
		fmt.Print("Enter interval (in ms): ")
		scanner := bufio.NewScanner(os.Stdin)
		for scanner.Scan() {
			if v, err := strconv.Atoi(scanner.Text()); err == nil {
				log.Printf("Setting new delay at %d milliseconds", v)
				ticker = time.Tick(time.Duration(v) * time.Millisecond)
				fmt.Print("Enter interval (in ms): ")
			} else {
				log.Printf("Invalid input")
				l.Disconnect()
				os.Exit(1)
			}
		}
		if err := scanner.Err(); err != nil {
			fmt.Fprintf(os.Stderr, "error reading standard input: %s", err)
		}
	}()

	var p int
	for {
		if p == *end {
			p = *start
		} else {
			p = *end
		}
		<-ticker
		l.Move(p, *speed)
	}
}

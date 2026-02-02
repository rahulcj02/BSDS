// hw3/go/context_switch/main.go
package main

import (
	"fmt"
	"runtime"
	"time"
)

const roundTrips = 1000000

func pingPong() (time.Duration, float64) {
	ch1 := make(chan struct{})
	ch2 := make(chan struct{})
	done := make(chan struct{})

	go func() {
		for i := 0; i < roundTrips; i++ {
			ch1 <- struct{}{}
			<-ch2
		}
		done <- struct{}{}
	}()

	go func() {
		for i := 0; i < roundTrips; i++ {
			<-ch1
			ch2 <- struct{}{}
		}
	}()

	start := time.Now()
	<-done
	total := time.Since(start)

	avgNs := float64(total.Nanoseconds()) / float64(2*roundTrips)
	return total, avgNs
}

func run(label string, procs int) {
	prev := runtime.GOMAXPROCS(procs)
	total, avgNs := pingPong()
	fmt.Printf("%s gomaxprocs=%d total=%s avg_switch_ns=%.2f (prev=%d)\n", label, procs, total, avgNs, prev)
}

func main() {
	run("one_thread", 1)
	run("multi_thread", runtime.NumCPU())
}

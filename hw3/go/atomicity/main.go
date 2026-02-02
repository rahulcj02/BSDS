package main

import (
	"fmt"
	"sync"
	"sync/atomic"
)

func main() {
	var ops uint64

	var wg sync.WaitGroup

	for i := 0; i < 50; i++ {
		wg.Add(1)

		go func() {
			for c := 0; c < 1000; c++ {
				atomic.AddUint64(&ops, 1)
			}
			wg.Done()
		}()
	}

	wg.Wait()

	fmt.Println("ops:", ops)

	// Non-atomic version to show race effects:
	var nonAtomic uint64
	wg = sync.WaitGroup{}

	for i := 0; i < 50; i++ {
		wg.Add(1)

		go func() {
			for c := 0; c < 1000; c++ {
				nonAtomic++
			}
			wg.Done()
		}()
	}

	wg.Wait()
	fmt.Println("nonAtomic:", nonAtomic)
}

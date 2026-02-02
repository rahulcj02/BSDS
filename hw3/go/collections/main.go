// hw3/go/collections/main.go
package main

import (
	"flag"
	"fmt"
	"sync"
	"time"
)

type LockedMap struct {
	mu sync.Mutex
	m  map[int]int
}

func (lm *LockedMap) Set(k, v int) {
	lm.mu.Lock()
	lm.m[k] = v
	lm.mu.Unlock()
}

func (lm *LockedMap) Len() int {
	lm.mu.Lock()
	n := len(lm.m)
	lm.mu.Unlock()
	return n
}

type RWLockedMap struct {
	mu sync.RWMutex
	m  map[int]int
}

func (rm *RWLockedMap) Set(k, v int) {
	rm.mu.Lock()
	rm.m[k] = v
	rm.mu.Unlock()
}

func (rm *RWLockedMap) Len() int {
	rm.mu.RLock()
	n := len(rm.m)
	rm.mu.RUnlock()
	return n
}

func main() {
	mode := flag.String("mode", "plain", "plain|mutex|rwmutex|syncmap")
	flag.Parse()

	switch *mode {
	case "plain":
		runPlain()
	case "mutex":
		runMutex()
	case "rwmutex":
		runRWMutex()
	case "syncmap":
		runSyncMap()
	default:
		fmt.Println("unknown mode:", *mode)
	}
}

func runPlain() {
	start := time.Now()

	m := map[int]int{}
	var wg sync.WaitGroup

	for g := 0; g < 50; g++ {
		wg.Add(1)
		go func(g int) {
			for i := 0; i < 1000; i++ {
				m[g*1000+i] = i
			}
			wg.Done()
		}(g)
	}

	wg.Wait()
	d := time.Since(start)

	fmt.Printf("mode=plain len=%d duration=%s\n", len(m), d)
}

func runMutex() {
	start := time.Now()

	lm := &LockedMap{m: map[int]int{}}
	var wg sync.WaitGroup

	for g := 0; g < 50; g++ {
		wg.Add(1)
		go func(g int) {
			for i := 0; i < 1000; i++ {
				lm.Set(g*1000+i, i)
			}
			wg.Done()
		}(g)
	}

	wg.Wait()
	d := time.Since(start)

	fmt.Printf("mode=mutex len=%d duration=%s\n", lm.Len(), d)
}

func runRWMutex() {
	start := time.Now()

	rm := &RWLockedMap{m: map[int]int{}}
	var wg sync.WaitGroup

	for g := 0; g < 50; g++ {
		wg.Add(1)
		go func(g int) {
			for i := 0; i < 1000; i++ {
				rm.Set(g*1000+i, i)
			}
			wg.Done()
		}(g)
	}

	wg.Wait()
	d := time.Since(start)

	fmt.Printf("mode=rwmutex len=%d duration=%s\n", rm.Len(), d)
}

func runSyncMap() {
	start := time.Now()

	var m sync.Map
	var wg sync.WaitGroup

	for g := 0; g < 50; g++ {
		wg.Add(1)
		go func(g int) {
			for i := 0; i < 1000; i++ {
				m.Store(g*1000+i, i)
			}
			wg.Done()
		}(g)
	}

	wg.Wait()

	count := 0
	m.Range(func(_, _ any) bool {
		count++
		return true
	})

	d := time.Since(start)

	fmt.Printf("mode=syncmap len=%d duration=%s\n", count, d)
}

// hw3/go/file_access/main.go
package main

import (
	"bufio"
	"fmt"
	"os"
	"time"
)

const iters = 100000

func unbuffered(path string) time.Duration {
	f, err := os.Create(path)
	if err != nil {
		panic(err)
	}
	defer f.Close()

	start := time.Now()
	for i := 0; i < iters; i++ {
		_, err := f.Write([]byte("hello\n"))
		if err != nil {
			panic(err)
		}
	}
	return time.Since(start)
}

func buffered(path string) time.Duration {
	f, err := os.Create(path)
	if err != nil {
		panic(err)
	}
	defer f.Close()

	w := bufio.NewWriter(f)
	start := time.Now()
	for i := 0; i < iters; i++ {
		_, err := w.WriteString("hello\n")
		if err != nil {
			panic(err)
		}
	}
	if err := w.Flush(); err != nil {
		panic(err)
	}
	return time.Since(start)
}

func main() {
	d1 := unbuffered("unbuffered.txt")
	d2 := buffered("buffered.txt")

	fmt.Printf("unbuffered=%s\n", d1)
	fmt.Printf("buffered=%s\n", d2)
}

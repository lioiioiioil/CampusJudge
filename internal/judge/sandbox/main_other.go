//go:build !linux

package main

import (
	"fmt"
	"os"
)

func main() {
	fmt.Fprintln(os.Stderr, "sandbox supervisor runs only in its Linux Docker image")
	os.Exit(1)
}

package main

import (
	"afk/internal/engine"
	"fmt"
	"os"
)

func main() {
	if err := engine.Run(os.Args[1:]); err != nil {
		fmt.Fprintln(os.Stderr, "afk:", err)
		os.Exit(1)
	}
}

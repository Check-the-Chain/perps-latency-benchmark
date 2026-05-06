package main

import (
	"fmt"
	"os"

	"perps-latency-benchmark/internal/app"
)

func main() {
	if err := app.NewRootCommand().Execute(); err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}
}

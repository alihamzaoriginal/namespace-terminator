package main

import (
	"os"

	"github.com/alihamzaoriginal/namespace-terminator/internal/cli"
)

func main() {
	if err := cli.Execute(); err != nil {
		os.Exit(1)
	}
}

package main

import (
	"os"
)

func main() {
	options.LoadConfig()

	if _, err := parser.Parse(); err != nil {
		os.Exit(1)
	}
}

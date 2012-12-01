package main

import (
	"os"
)

func main() {
	parser.ApplicationName = "autobuild"

	parser.WriteManPage(os.Stdout,
	                    "Autobuild description")
}

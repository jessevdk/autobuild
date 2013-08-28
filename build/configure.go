package main

import (
	"github.com/jessevdk/go-configure"
)

func main() {
	configure.Version = []int {0, 1}
	configure.Target = "autobuild"

	configure.Configure(nil)
}

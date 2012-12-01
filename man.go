package main

import (
	"os"
)

func main() {
	parser.ApplicationName = "autobuild"

	parser.WriteManPage(os.Stdout,
	                    "autobuild is an easy to use debian package build system. It's main purpose is to simplify the process of maintaing various pieces of the debian package build system (mainly pbuilder and reprepro). autobuild provides a single entry point to managing the build process.")
}

package main

import (
	"fmt"
	"os"
	"path"
)

type CommandInstall struct {
}

func (x *CommandInstall) Execute(args []string) error {
	pkgs := []string{
		"cowbuilder",
		"devscripts",
		"reprepro",
		"debootstrap",
		"debian-archive-keyring",
		"ubuntu-keyring",
		"bzip2",
		"gzip",
		"xz-utils",
		"patch",
	}

	aptargs := []string{
		"install",
		"-y",
	}

	if !options.Verbose {
		aptargs = append(aptargs, "-q")
	}

	fmt.Println("Installing dependencies")
	RunCommand("apt-get", append(aptargs, pkgs...)...)

	// Extract pbuilderrc
	fmt.Printf("Copying configuration files to `%s'\n", options.Base)

	WriteResource("pbuilderrc", path.Join(options.Base, "etc", "pbuilderrc"))

	updatehook := path.Join(options.Base, "pbuilder", "hooks", "D10apt-get-update")
	WriteResource("D10apt-get-update", updatehook)

	// Make hook executable
	os.Chmod(updatehook, 0755)

	// Create dirs
	for _, dir := range []string{"repository", "pbuilder"} {
		os.MkdirAll(path.Join(options.Base, dir), 0755)
	}

	for _, dir := range []string{"ccache", "aptcache", "result", "hooks"} {
		os.MkdirAll(path.Join(options.Base, "pbuilder", dir), 0755)
	}

	return nil
}

func init() {
	parser.AddCommand("Install", "install", &CommandInstall{})
}

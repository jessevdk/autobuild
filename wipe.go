package main

import (
	"os"
)

type CommandWipe struct {
}

func (x *CommandWipe) Execute(args []string) error {
	if options.Base != "" && options.Base != "/" {
		return os.RemoveAll(options.Base)
	}

	return nil
}

func init() {
	parser.AddCommand("Wipe", "wipe", &CommandWipe{})
}

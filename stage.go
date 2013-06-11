package main

import (
	"io/ioutil"
)

type CommandStage struct {
}

func (x *CommandStage) Execute(args []string) error {
	// Stage all packages listed in 'args'
	for _, arg := range args {
		data, err := ioutil.ReadFile(arg)

		if err != nil {
			return err
		}

		a := &Stage{
			Filename: arg,
			Data:     data,
		}

		ret := &StageReply{}

		if err := RemoteCall("DaemonCommands.Stage", a, ret); err != nil {
			return err
		}
	}

	return nil
}

func init() {
	parser.AddCommand("stage",
		"Stage a package to be built in the build daemon",
		"The stage command stages a package to be built. The staged package has a very specific layout. If your package original tarball is named example-1.0.tar.gz, then the autobuild package needs to be named example_1.0.tar.gz and contain example_1.0.orig.tar.gz and example_1.0.diff.gz. An optional patches/ directory may contain distribution specific patches (e.g. lucid.gz, precise.gz) to be applied per distribution.",
		&CommandStage{})
}

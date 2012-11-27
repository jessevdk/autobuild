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

		a := &Archive{
			Filename: arg,
			Data:     data,
		}

		ret := &GeneralReply{}

		if err := RemoteCall("DaemonCommands.Stage", a, ret); err != nil {
			return err
		}
	}

	return nil
}

func init() {
	parser.AddCommand("Stage", "stage", &CommandStage{})
}

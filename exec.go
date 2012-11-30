package main

import (
	"fmt"
	"os"
	"os/exec"
)

func prepareCommand(name string, arg ...string) *exec.Cmd {
	cmd := exec.Command(name, arg...)

	if options.Verbose {
		cmd.Stdout = os.Stdout
		cmd.Stderr = os.Stderr
	}

	return cmd
}

func runCommandReal(cmd *exec.Cmd) error {
	if err := cmd.Run(); err != nil {
		fmt.Fprintf(os.Stderr, "Error running command: %s\n", err)
		return err
	}

	return nil
}

func RunCommand(name string, arg ...string) error {
	return runCommandReal(MakeCommand(name, arg...))
}

func RunOutputCommand(name string, arg ...string) ([]byte, error) {
	cmd := MakeCommand(name, arg...)
	cmd.Stdout = nil

	return cmd.Output()
}

func RunCommandIn(wd string, name string, arg ...string) error {
	return runCommandReal(MakeCommandIn(wd, name, arg...))
}

func MakeCommand(name string, arg ...string) *exec.Cmd {
	return prepareCommand(name, arg...)
}

func MakeCommandIn(wd string, name string, arg ...string) *exec.Cmd {
	ret := prepareCommand(name, arg...)

	ret.Dir = wd

	return ret
}

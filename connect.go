package main

import (
	"io"
	"net"
	"os"
	"path"
)

type CommandConnect struct {
}

func (x *CommandConnect) Execute(args []string) error {
	// Connect to the local autobuild socket and act as gateway
	cl, err := net.Dial("unix", path.Join(options.Base, "run", "autobuild.sock"))

	if err != nil {
		return err
	}

	if err := RemoteSendCredentials(cl.(*net.UnixConn)); err != nil {
		return err
	}

	go func() {
		io.Copy(cl, os.Stdin)
		cl.Close()
	}()

	_, err = io.Copy(os.Stdout, cl)

	// Wait for both to finish
	return err
}

func init() {
	parser.AddCommand("Connect", "connect", &CommandConnect{})
}

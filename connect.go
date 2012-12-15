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
	parser.AddCommand("connect",
		"Connect to a the autobuild socket and relay stdin",
		"The connect command connects to a autobuild socket and then relays all data on standard in to this connection. When using a remote connection (-r, --remote) for client commands (such as stage or release), a ssh connection is made to the remote and `autobuild connect' is executed allowing the remote call with proper authentication.",
		&CommandConnect{})
}

package main

import (
	"fmt"
	"io"
	"net"
	"net/rpc"
	"path"
)

type PipesReadWrite struct {
	Stdin  io.ReadCloser
	Stdout io.WriteCloser
}

func (x *PipesReadWrite) Read(p []byte) (n int, err error) {
	return x.Stdin.Read(p)
}

func (x *PipesReadWrite) Write(p []byte) (n int, err error) {
	return x.Stdout.Write(p)
}

func (x *PipesReadWrite) Close() error {
	if err := x.Stdin.Close(); err != nil {
		return err
	}

	if err := x.Stdout.Close(); err != nil {
		return err
	}

	return nil
}

func RemoteConnect(socketfile string) (io.ReadWriteCloser, error) {
	if options.Remote == "" {
		if len(socketfile) == 0 {
			socketfile = path.Join(options.Base, "run", "autobuild.sock")
		}

		cl, err := net.Dial("unix", socketfile)

		if err != nil {
			return nil, err
		}

		if err := RemoteSendCredentials(cl.(*net.UnixConn)); err != nil {
			return nil, err
		}

		return cl, nil
	} else {
		// Connect to the remote using ssh
		scmd := "autobuild connect"

		if len(socketfile) != 0 {
			scmd = scmd + " " + socketfile
		}

		cmd := MakeCommand("ssh", options.Remote, scmd)
		inp, err := cmd.StdinPipe()

		if err != nil {
			return nil, err
		}

		outp, err := cmd.StdoutPipe()

		if err != nil {
			return nil, err
		}

		rw := &PipesReadWrite{
			Stdin:  outp,
			Stdout: inp,
		}

		if err := cmd.Start(); err != nil {
			return nil, err
		}

		return rw, nil
	}

	return nil, nil
}

func RemoteCall(method string, args interface{}, reply interface{}) error {
	rwc, err := RemoteConnect("")

	if err != nil {
		if options.Verbose {
			fmt.Printf("Failed to connect: %s\n", err)
		}

		return err
	}

	c := rpc.NewClient(rwc)

	if options.Verbose {
		fmt.Printf("Connected to remote daemon\n")
	}

	if err := c.Call(method, args, reply); err != nil {
		fmt.Printf("Failed to call remote method %v: %s\n", method, err)
		return err
	}

	c.Close()
	return nil
}

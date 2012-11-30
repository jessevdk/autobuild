package main

import (
	"io"
	"net"
	"net/rpc"
	"os"
	"path"
	"reflect"
	"syscall"
	"fmt"
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

func RemoteSendCredentials(conn *net.UnixConn) error {
	ucred := &syscall.Ucred{
		Pid: int32(os.Getpid()),
		Uid: uint32(os.Getuid()),
		Gid: uint32(os.Getgid()),
	}

	oob := syscall.UnixCredentials(ucred)
	_, _, err := conn.WriteMsgUnix(nil, oob, nil)

	return err
}

func sysfd(c net.Conn) int {
	cv := reflect.ValueOf(c)

	switch ce := cv.Elem(); ce.Kind() {
	case reflect.Struct:
		netfd := ce.FieldByName("fd")

		switch fe := netfd.Elem(); fe.Kind() {
		case reflect.Struct:
			fd := fe.FieldByName("sysfd")
			return int(fd.Int())
		}
	}

	return 0
}

func RemoteRecvCredentials(conn *net.UnixConn) (uint32, uint32, error) {
	err := syscall.SetsockoptInt(sysfd(conn), syscall.SOL_SOCKET, syscall.SO_PASSCRED, 1)

	if err != nil {
		return 0, 0, err
	}

	oob := make([]byte, len(syscall.UnixCredentials(&syscall.Ucred{})))

	_, _, _, _, err = conn.ReadMsgUnix(nil, oob)

	if err != nil {
		return 0, 0, err
	}

	scm, err := syscall.ParseSocketControlMessage(oob)

	if err != nil {
		return 0, 0, err
	}

	ucred, err := syscall.ParseUnixCredentials(&scm[0])

	if err != nil {
		return 0, 0, err
	}

	return ucred.Uid, ucred.Gid, nil
}

func RemoteConnect() (*rpc.Client, error) {
	if options.Remote == "" {
		cl, err := net.Dial("unix", path.Join(options.Base, "run", "autobuild.sock"))

		if err != nil {
			return nil, err
		}

		if err := RemoteSendCredentials(cl.(*net.UnixConn)); err != nil {
			return nil, err
		}

		return rpc.NewClient(cl), nil
	} else {
		// Connect to the remote using ssh
		cmd := MakeCommand("ssh", options.Remote, "autobuild connect")
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

		return rpc.NewClient(rw), nil
	}

	return nil, nil
}

func RemoteCall(method string, args interface{}, reply interface{}) error {
	c, err := RemoteConnect()

	if err != nil {
		if options.Verbose {
			fmt.Printf("Failed to connect: %s\n", err)
		}

		return err
	}

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

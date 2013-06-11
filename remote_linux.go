// +build linux

package main

import (
	"net"
	"os"
	"reflect"
	"syscall"
)

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

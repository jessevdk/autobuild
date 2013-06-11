// +build !linux

package main

import (
	"net"
)

func RemoteSendCredentials(conn *net.UnixConn) error {
	return nil
}

func RemoteRecvCredentials(conn *net.UnixConn) (uint32, uint32, error) {
	return 0, 0, nil
}

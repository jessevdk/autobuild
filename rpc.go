package main

import (
	"bufio"
	"encoding/gob"
	"fmt"
	"io"
	"net"
	"net/rpc"
	"os"
	"path"
	"reflect"
)

type CodecWithAuth struct {
	rwc    io.ReadWriteCloser
	dec    *gob.Decoder
	enc    *gob.Encoder
	encBuf *bufio.Writer

	Uid uint32
}

func (c *CodecWithAuth) ReadRequestHeader(r *rpc.Request) error {
	return c.dec.Decode(r)
}

func (c *CodecWithAuth) ReadRequestBody(body interface{}) error {
	if err := c.dec.Decode(body); err != nil {
		return err
	}

	v := reflect.ValueOf(body)

	if v.Kind() == reflect.Ptr && !v.IsNil() {
		v = v.Elem()
	}

	if v.Kind() == reflect.Struct {
		field := v.FieldByName("Uid")

		if !field.IsValid() || field.Kind() != reflect.Uint32 {
			return nil
		}

		field.SetUint(uint64(c.Uid))
	}

	return nil
}

func (c *CodecWithAuth) WriteResponse(r *rpc.Response, body interface{}) (err error) {
	if err = c.enc.Encode(r); err != nil {
		return
	}

	if err = c.enc.Encode(body); err != nil {
		return
	}

	return c.encBuf.Flush()
}

func (c *CodecWithAuth) Close() error {
	return c.rwc.Close()
}

func (x *CommandDaemon) listenRpc() (*rpc.Server, error) {
	dirname := path.Join(options.Base, "run")
	os.MkdirAll(dirname, 0755)

	spath := path.Join(dirname, "autobuild.sock")
	listener, err := net.Listen("unix", spath)

	if err != nil {
		fmt.Errorf("Failed to create remote listen socket on `%s': %s\n",
			spath,
			err)

		return nil, err
	}

	os.Chmod(spath, 0777)

	cmds := &DaemonCommands{}

	server := rpc.NewServer()
	server.Register(cmds)

	go func() {
		ul := listener.(*net.UnixListener)

		for {
			cl, err := ul.AcceptUnix()

			if err != nil {
				break
			}

			uid, _, err := RemoteRecvCredentials(cl)

			if err != nil {
				if options.Verbose {
					fmt.Printf("Failed to verify credentials: %s\n", err)
				}

				cl.Close()
				continue
			}

			if !x.verifyCredentials(uid) {
				if options.Verbose {
					fmt.Printf("User is not authenticated, closing connection...\n")
				}

				cl.Close()
			} else {
				buf := bufio.NewWriter(cl)
				srv := &CodecWithAuth{cl, gob.NewDecoder(cl), gob.NewEncoder(buf), buf, uid}

				go server.ServeCodec(srv)
			}
		}
	}()

	return server, nil
}

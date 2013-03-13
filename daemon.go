package main

import (
	"errors"
	"fmt"
	"os"
	"os/signal"
	"os/user"
	"path"
	"syscall"
)

type CommandDaemon struct {
}

func (x *CommandDaemon) verifyCredentials(uid uint32) bool {
	if len(options.Group) != 0 {
		us, err := user.LookupId(fmt.Sprintf("%v", uid))

		if err != nil {
			return false
		}

		return userIsMemberOfGroup(us.Username, options.Group)
	}

	return true
}

func (x *CommandDaemon) Execute(args []string) error {
	builder.Load()
	defer builder.Save()

	syscall.RawSyscall(syscall.SYS_IOCTL, 0, uintptr(syscall.TIOCNOTTY), 0)

	// Run remote socket
	if _, err := x.listenRpc(); err != nil {
		return err
	}

	// Run repository http server
	if err := x.listenRepository(); err != nil {
		return err
	}

	sig := make(chan os.Signal, 10)
	signal.Notify(sig, syscall.SIGINT, syscall.SIGTERM)

	defer os.Remove(path.Join(options.Base, "run", "autobuild.sock"))
	go builder.Run()

	for {
		select {
		case s := <-sig:
			if s == syscall.SIGINT {
				return errors.New("")
			}

			return nil
		}
	}

	return nil
}

func init() {
	parser.AddCommand("daemon",
		"Run the autobuild build daemon",
		"The daemon command runs the autobuild build daemon. The build daemon performs several tasks. First, it manages the package queue and listens for client commands to stage or release packages. It also runs a webserver serving the repository contents over http.",
		&CommandDaemon{})
}

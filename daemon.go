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
	if err := builder.Load(); err != nil {
		fmt.Fprintf(os.Stderr, "Failed to load builder state: %s\n", err)
	}

	defer func() {
		if err := builder.Save(); err != nil {
			fmt.Fprintf(os.Stderr, "Failed to save builder state: %s\n", err)
		}
	}()

	syscall.RawSyscall(syscall.SYS_IOCTL, 0, uintptr(syscall.TIOCNOTTY), 0)

	// Run remote socket
	if _, err := x.listenRpc(); err != nil {
		return err
	}

	// Run repository http server
	if err := x.listenRepository(); err != nil {
		return err
	}

	// Setup tmpfs if needed
	if options.UseTmpfs {
		builddir := path.Join(options.Base, "pbuilder", "build")

		os.MkdirAll(builddir, 0755)

		if err := RunCommand("mount", "-t", "tmpfs", "tmpfs", builddir); err == nil {
			defer RunCommand("umount", builddir)
		}
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

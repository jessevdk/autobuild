package main

import (
	"errors"
	"fmt"
	"os"
	"os/signal"
	"path"
	"syscall"
	"os/user"
	"encoding/json"
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

type State struct {
	BuildInfo []*BuildInfo
	Queue []*PackageInfo
}

func (x *CommandDaemon) loadState() {
	filename := path.Join(options.Base, "run", "autobuild.state")
	f, err := os.Open(filename)

	if err != nil {
		return
	}

	defer f.Close()

	state := &State {}

	dec := json.NewDecoder(f)

	if err := dec.Decode(state); err != nil {
		return
	}

	resultsMutex.Lock()
	results = state.BuildInfo
	resultsMutex.Unlock()

	for _, q := range state.Queue {
		queue <- q
	}
}

func (x *CommandDaemon) saveState() {
	filename := path.Join(options.Base, "run", "autobuild.state")
	f, err := os.Create(filename)

	if err != nil {
		return
	}

	defer f.Close()

	q := make([]*PackageInfo, 0)
	close(queue)

	if building != nil {
		q = append(q, building)
	}

	for info := range queue {
		q = append(q, info)
	}

	resultsMutex.Lock()
	defer resultsMutex.Unlock()

	state := &State {
		BuildInfo: results,
		Queue: q,
	}

	enc := json.NewEncoder(f)

	if err := enc.Encode(state); err != nil {
		return
	}
}

func (x *CommandDaemon) Execute(args []string) error {
	x.loadState()
	defer x.saveState()

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

	for {
		select {
		case pack := <-queue:
			if err := buildPackage(pack); err != nil {
				if options.Verbose {
					fmt.Printf("Error during building of `%s': %s...\n",
					           pack.Name,
					           err)
				}
			} else if options.Verbose {
				fmt.Printf("Finished build of `%s'...\n", pack.Name)
			}

			os.Remove(pack.StageFile)
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

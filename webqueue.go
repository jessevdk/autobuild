package main

import (
	"encoding/json"
	"fmt"
	"io"
	"net"
	"net/http"
	"os"
	"os/exec"
	"os/signal"
	"path"
	"syscall"
)

type WebQueueCommand struct {
}

type ResourceFS struct {
	Prefix string
}

func (x *ResourceFS) Open(name string) (http.File, error) {
	res, err := GetResource(path.Join(x.Prefix, name))

	if err != nil {
		return nil, err
	}

	return res, nil
}

func WebQueueServiceHandle(w http.ResponseWriter, r *http.Request, uid uint32) {
	if r.URL.Path == "/queue" {
		w.Header().Add("Content-type", "application/json")

		// Index
		builder.Do(func(b *PackageBuilder) error {
			packages := make([]*BuildInfo, 0)

			for _, res := range b.FinishedPackages {
				if res.Info.Uid != uid {
					continue
				}

				packages = append(packages, res)
			}

			enc := json.NewEncoder(w)

			enc.Encode(map[string][]*BuildInfo{
				"packages": packages,
			})

			return nil
		})
	} else if r.URL.Path == "/queue/release" || r.URL.Path == "/queue/discard" {
		packages := make([]uint64, 0)

		dec := json.NewDecoder(r.Body)
		dec.Decode(&packages)

		var retpack []uint64
		var err error

		if r.URL.Path == "/queue/release" {
			retpack, err = builder.Release(packages)
		} else if r.URL.Path == "/queue/discard" {
			retpack, err = builder.Discard(packages)
		}

		enc := json.NewEncoder(w)

		ret := struct {
			Packages []uint64
			Error    error
		}{
			Packages: retpack,
			Error:    err,
		}

		w.Header().Add("Content-type", "application/json")
		enc.Encode(ret)
	}
}

func RunWebQueueService(filename string, uid uint32) (chan bool, error) {
	// Run a http server on 'filename' unix socket
	ln, err := net.Listen("unix", filename)

	if err != nil {
		return nil, err
	}

	os.Chmod(filename, 0777)

	mux := http.NewServeMux()

	/*mux.Handle("/", http.FileServer(&ResourceFS{
		Prefix: "webqueue",
	}))*/

	mux.Handle("/", http.FileServer(http.Dir("resources/webqueue")))

	mux.HandleFunc("/queue", func(w http.ResponseWriter, r *http.Request) {
		WebQueueServiceHandle(w, r, uid)
	})

	serv := &http.Server{
		Handler: mux,
	}

	closer := make(chan bool, 1)

	go func() {
		serv.Serve(ln)

		ln.Close()
		os.Remove(filename)
	}()

	go func() {
		select {
		case <-closer:
			ln.Close()
		}
	}()

	return closer, nil
}

func (x *WebQueueCommand) Execute(args []string) error {
	// Request socket from daemon
	srv := &WebQueueService{}
	repl := &WebQueueReply{}

	if err := RemoteCall("DaemonCommands.WebQueueService", srv, repl); err != nil {
		return err
	}

	// Launch local tcp to forward connections on the remote connect
	ln, err := net.Listen("tcp", ":0")

	if err != nil {
		return err
	}

	port := ln.Addr().(*net.TCPAddr).Port
	finished := make(chan bool, 1)

	go func() {
		for {
			conn, err := ln.Accept()

			if err != nil {
				fmt.Fprintf(os.Stderr, "Failed to accept connection: %s\n", err)
				continue
			}

			go func() {
				// Create remote connect
				cl, err := RemoteConnect(repl.SocketFile)

				if err != nil {
					fmt.Fprintf(os.Stderr, "Failed to connect remote: %s\n", err)
					return
				}

				ff := make(chan bool, 2)

				// Forward data over the remote connection
				go func() {
					io.Copy(cl, conn)
					ff <- true
				}()

				go func() {
					io.Copy(conn, cl)
					ff <- true
				}()

				<-ff
				<-ff

				cl.Close()
				conn.Close()
			}()
		}

		finished <- true
	}()

	go exec.Command("x-www-browser", fmt.Sprintf("http://localhost:%v/queue.html", port)).Run()

	sig := make(chan os.Signal, 10)
	signal.Notify(sig, syscall.SIGINT, syscall.SIGTERM)

	select {
	case <-sig:
		break
	case <-finished:
		break
	}

	// Send close webservice call
	RemoteCall("DaemonCommands.CloseWebQueueService", repl, &GeneralReply{})
	return nil
}

func init() {
	parser.AddCommand("webqueue",
		"Launch a local browser showing the current autobuild queue",
		"The webqueue command launches a local browser showing the current autobuild queue. The provided website allows you to stage, release and discard packages and look at build logs of failed packages.",
		&WebQueueCommand{})
}

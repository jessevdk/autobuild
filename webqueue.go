package main

import (
	"compress/gzip"
	"encoding/json"
	"fmt"
	"io"
	"mime/multipart"
	"net"
	"net/http"
	"os"
	"os/signal"
	"path"
	"strconv"
	"strings"
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

func WebQueueServiceHandleQueue(w http.ResponseWriter, r *http.Request, uid uint32) {
	w.Header().Add("Content-type", "application/json")

	// Index
	builder.Do(func(b *PackageBuilder) error {
		packages := make([]*BuildInfo, 0)

		for _, res := range b.FinishedPackages {
			if res.Info.Uid == uid {
				packages = append(packages, res)
			}
		}

		enc := json.NewEncoder(w)

		return enc.Encode(map[string]interface{}{
			"packages": packages,
			"building": b.CurrentlyBuilding,
			"queue":    b.PackageQueue,
		})
	})
}

func decodeWebPackages(r *http.Request, prefix string, uid uint32) []uint64 {
	packages := make([]uint64, 0)

	rest := r.URL.Path[len(prefix):]

	json.Unmarshal([]byte(rest), &packages)
	return packages
}

func encodeWebPackages(w http.ResponseWriter, pkgs []uint64, err error) {
	enc := json.NewEncoder(w)

	ret := struct {
		Packages []uint64
		Error    error
	}{
		Packages: pkgs,
		Error:    err,
	}

	w.Header().Add("Content-type", "application/json")
	enc.Encode(ret)
}

func WebQueueServiceHandleRelease(w http.ResponseWriter, r *http.Request, uid uint32) {
	pkgs, err := builder.Release(decodeWebPackages(r, "/queue/release/", uid), uid)
	encodeWebPackages(w, pkgs, err)
}

func WebQueueServiceHandleDiscard(w http.ResponseWriter, r *http.Request, uid uint32) {
	pkgs, err := builder.Discard(decodeWebPackages(r, "/queue/discard/", uid), uid)
	encodeWebPackages(w, pkgs, err)
}

func WebQueueServiceHandleDownload(w http.ResponseWriter, r *http.Request, uid uint32) {
	downprefix := "/queue/download/"

	rest := r.URL.Path[len(downprefix):]
	parts := strings.SplitN(rest, "/", 2)

	if len(parts) != 2 {
		return
	}

	id, err := strconv.ParseUint(parts[0], 10, 64)

	if err != nil {
		return
	}

	file := parts[1]

	var pkg *DistroBuildInfo

	// Find package with this id
	if err := builder.Do(func(b *PackageBuilder) error {
		_, pkg = b.FindPackage(id)
		return nil
	}); err != nil {
		return
	}

	if pkg == nil {
		return
	}

	if strings.HasSuffix(file, ".dsc") || strings.HasSuffix(file, ".changes") {
		w.Header().Add("Content-type", "text/plain")
	} else {
		w.Header().Add("Content-disposition",
			fmt.Sprintf("attachment; filename=\"%s\"",
				file))

		if strings.HasSuffix(file, ".tar.gz") {
			w.Header().Add("Content-type", "application/x-gzip-compressed-tar")
		} else if strings.HasSuffix(file, ".gz") {
			w.Header().Add("Content-type", "application/x-gzip")
		} else if strings.HasSuffix(file, ".deb") {
			w.Header().Add("Content-type", "application/x-deb")
		}
	}

	for _, f := range pkg.Files {

		if path.Base(f) == file {
			rd, err := os.Open(f)

			if err == nil {
				io.Copy(w, rd)
				rd.Close()
			}

			break
		}
	}
}

func WebQueueServiceHandleLog(w http.ResponseWriter, r *http.Request, uid uint32) {
	logprefix := "/queue/log/"

	id, err := strconv.ParseUint(r.URL.Path[len(logprefix):], 10, 64)

	if err != nil {
		return
	}

	var pkg *DistroBuildInfo

	// Find package with this id
	if err := builder.Do(func(b *PackageBuilder) error {
		_, pkg = b.FindPackage(id)
		return nil
	}); err != nil {
		return
	}

	if pkg != nil {
		w.Header().Add("Content-type", "text/plain")
		io.WriteString(w, pkg.Log)
	}
}

func WebQueueStage(file *multipart.FileHeader, uid uint32) (*PackageInfo, error) {
	return builder.Stage(file.Filename, uid, func(b *PackageBuilder, writer io.Writer) error {
		f, err := file.Open()

		if err != nil {
			return err
		}

		defer f.Close()
		_, err = io.Copy(writer, f)

		return err
	})
}

func WebQueueServiceHandleStage(w http.ResponseWriter, r *http.Request, uid uint32) {
	if err := r.ParseMultipartForm(0); err != nil {
		return
	}

	type WebStageReply struct {
		Info  *PackageInfo
		Error error
	}

	ret := make(map[string]WebStageReply)

	for name, headers := range r.MultipartForm.File {
		for _, header := range headers {
			info, err := WebQueueStage(header, uid)

			ret[name] = WebStageReply{
				info,
				WrapError(err),
			}
		}
	}

	enc := json.NewEncoder(w)

	w.Header().Add("Content-type", "application/json")
	enc.Encode(ret)
}

type CompressedFileServer struct {
	http.Handler
}

type CompressedResponseWriter struct {
	http.ResponseWriter

	gwriter *gzip.Writer
}

func (x *CompressedResponseWriter) Write(data []byte) (int, error) {
	return x.gwriter.Write(data)
}

func (x *CompressedFileServer) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	if strings.Contains(r.Header.Get("Accept-Encoding"), "gzip") {
		w.Header().Set("Content-Encoding", "gzip")
		x.Handler.ServeHTTP(w, r)
	} else {
		c := &CompressedResponseWriter{
			ResponseWriter: w,
			gwriter:        gzip.NewWriter(w),
		}

		x.Handler.ServeHTTP(c, r)
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

	mux.Handle("/", &CompressedFileServer{
		Handler: http.FileServer(&ResourceFS{
			Prefix: "webqueue",
		}),
	})

	mux.HandleFunc("/queue/", func(w http.ResponseWriter, r *http.Request) {
		WebQueueServiceHandleQueue(w, r, uid)
	})

	mux.HandleFunc("/queue/log/", func(w http.ResponseWriter, r *http.Request) {
		WebQueueServiceHandleLog(w, r, uid)
	})

	mux.HandleFunc("/queue/download/", func(w http.ResponseWriter, r *http.Request) {
		WebQueueServiceHandleDownload(w, r, uid)
	})

	mux.HandleFunc("/queue/stage", func(w http.ResponseWriter, r *http.Request) {
		WebQueueServiceHandleStage(w, r, uid)
	})

	mux.HandleFunc("/queue/discard/", func(w http.ResponseWriter, r *http.Request) {
		WebQueueServiceHandleDiscard(w, r, uid)
	})

	mux.HandleFunc("/queue/release/", func(w http.ResponseWriter, r *http.Request) {
		WebQueueServiceHandleRelease(w, r, uid)
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

	url := fmt.Sprintf("http://localhost:%v/queue.html", port)
	x.openBrowser(url)

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

package main

import (
	"fmt"
	"net/http"
	"os"
	"path"
	"strings"
	"time"
)

func fileIsHidden(name string) bool {
	b := path.Join(options.Base, "repository")

	if strings.HasPrefix(name, b) {
		name = name[len(b):]
	}

	if len(name) == 0 || name[0] != '/' {
		name = fmt.Sprintf("/%s", name)
	}

	parts := strings.SplitN(name, "/", 4)
	return len(parts) > 2 && (parts[2] == "incoming" || parts[2] == "db" || parts[2] == "conf")
}

type HideReaddirFile struct {
	http.File
}

func (f HideReaddirFile) Readdir(count int) ([]os.FileInfo, error) {
	osf, ok := f.File.(*os.File)

	if ok && fileIsHidden(osf.Name()) {
		return nil, os.ErrNotExist
	}

	ret, err := f.File.Readdir(count)

	if err != nil {
		return nil, err
	}

	filt := make([]os.FileInfo, 0, len(ret))

	for _, info := range ret {
		name := path.Join(osf.Name(), info.Name())

		if !fileIsHidden(name) {
			filt = append(filt, info)
		}
	}

	return filt, nil
}

type RepositoryFS struct {
	fs http.FileSystem
}

func (fs RepositoryFS) Open(name string) (http.File, error) {
	if fileIsHidden(name) {
		return nil, os.ErrNotExist
	}

	f, err := fs.fs.Open(name)

	if err != nil {
		return nil, err
	}

	return HideReaddirFile{f}, nil
}

func (x *CommandDaemon) listenRepository() error {
	d := http.Dir(path.Join(options.Base, "repository"))
	fs := http.FileServer(RepositoryFS{d})

	s := &http.Server{
		Addr:         fmt.Sprintf(":%s", options.Repository.ListenPort),
		Handler:      fs,
		ReadTimeout:  10 * time.Second,
		WriteTimeout: 10 * time.Second,
	}

	go s.ListenAndServe()
	return nil
}

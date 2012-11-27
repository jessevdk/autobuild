package main

import (
	"path"
	"os"
)

type DaemonCommands struct {
}

type Archive struct {
	Filename string
	Data     []byte
}

type GeneralReply struct {
}

func (x *DaemonCommands) Stage(archive *Archive, reply *GeneralReply) error {
	// Create stage dir if necessary
	stagedir := path.Join(options.Base, "stage")
	os.MkdirAll(stagedir, 0755)

	// Write stage file data there
	basename := path.Base(archive.Filename)
	full := path.Join(stagedir, basename)

	f, err := os.OpenFile(full, os.O_CREATE | os.O_EXCL | os.O_WRONLY, 0644)

	if err != nil {
		return err
	}

	_, err = f.Write(archive.Data)

	f.Close()

	if err != nil {
		os.Remove(full)
		return err
	}

	queue <- NewPackageInfo(full)
	return nil
}

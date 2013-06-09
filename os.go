package main

import (
	"io"
	"os"
	"path"
)

func MoveFile(source string, dest string) error {
	os.MkdirAll(path.Dir(dest), 0755)

	// First try renaming the file
	if err := os.Rename(source, dest); err == nil {
		return nil
	}

	// Try copy instead
	fr, err := os.Open(source)

	if err != nil {
		return err
	}

	defer fr.Close()

	fw, err := os.Create(dest)

	if err != nil {
		return err
	}

	_, err = io.Copy(fw, fr)

	fw.Close()

	// Remove source after copy
	fr.Close()

	if err != nil {
		return err
	}

	return os.Remove(source)
}

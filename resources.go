package main

import (
	"debug/elf"
	"fmt"
	"os"
	"path"
)

var fd *elf.File

func init() {
	var err error

	fd, err = elf.Open(os.Args[0])

	if err != nil {
		fmt.Fprintf(os.Stderr,
			"Failed to open file `%s' for resources.\n",
			os.Args[0])
	}
}

func GetResource(name string) []byte {
	section := fd.Section("autobuild_res_" + name)

	if section == nil {
		return nil
	}

	data, err := section.Data()

	if err != nil {
		fmt.Fprintf(os.Stderr,
			"Failed to read resource `%s'.\n",
			name)

		return nil
	}

	return data
}

func WriteResource(name string, target string) {
	data := GetResource(name)

	if data == nil {
		return
	}

	os.MkdirAll(path.Dir(target), 0755)

	f, err := os.Create(target)

	if err != nil {
		fmt.Fprintf(os.Stderr,
			"Failed to write resource `%s' to `%s': %s.\n",
			name,
			target,
			err)

		return
	}

	if _, err := f.Write(data); err != nil {
		fmt.Fprintf(os.Stderr,
			"Failed to write resource `%s' to `%s': %s.\n",
			name,
			target,
			err)
	}

	f.Close()
}

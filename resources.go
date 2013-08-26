package main

import (
	"fmt"
	"os"
	"path"
	"time"
)

type ResourceFile struct {
	Data   []byte
	Offset int64

	name string
}

func (x *ResourceFile) Close() error {
	x.Offset = 0
	return nil
}

func (x *ResourceFile) Stat() (os.FileInfo, error) {
	return x, nil
}

func (x *ResourceFile) Readdir(count int) ([]os.FileInfo, error) {
	return nil, nil
}

func (x *ResourceFile) Read(ret []byte) (int, error) {
	return copy(ret, x.Data[x.Offset:]), nil
}

func (x *ResourceFile) Name() string {
	return x.name
}

func (x *ResourceFile) Size() int64 {
	return int64(len(x.Data))
}

func (x *ResourceFile) Mode() os.FileMode {
	return 0644
}

func (x *ResourceFile) ModTime() time.Time {
	return time.Now()
}

func (x *ResourceFile) IsDir() bool {
	return false
}

func (x *ResourceFile) Sys() interface{} {
	return nil
}

func (x *ResourceFile) Seek(offset int64, whence int) (int64, error) {
	switch whence {
	case os.SEEK_SET:
		x.Offset = offset
	case os.SEEK_CUR:
		x.Offset += offset
	case os.SEEK_END:
		x.Offset = int64(len(x.Data)) + offset
	}

	return x.Offset, nil
}

func GetResource(name string) (*ResourceFile, error) {
	data, ok := Resources["/"+name]

	if !ok {
		return nil, os.ErrNotExist
	}

	return &ResourceFile{
		name: name,
		Data: data,
	}, nil
}

func WriteResource(name string, target string) {
	res, err := GetResource(name)

	if res == nil {
		fmt.Fprintf(os.Stderr,
		            "Failed to obtain resource `%s' to `%s': %s\n",
		            name,
		            target,
		            err)
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

	if _, err := f.Write(res.Data); err != nil {
		fmt.Fprintf(os.Stderr,
			"Failed to write resource `%s' to `%s': %s.\n",
			name,
			target,
			err)
	}

	f.Close()
}

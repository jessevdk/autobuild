package main

import (
	"bytes"
	"encoding/binary"
	"os"
	"syscall"
	"unsafe"
)

// #include <linux/inotify.h>
// #include <stdio.h>
// #include <stdlib.h>
import "C"

type WatchMask uint32

const (
	WatchAccess       WatchMask = C.IN_ACCESS
	WatchModify                 = C.IN_MODIFY
	WatchAttrib                 = C.IN_ATTRIB
	WatchCloseWrite             = C.IN_CLOSE_WRITE
	WatchCloseNoWrite           = C.IN_CLOSE_NOWRITE
	WatchOpen                   = C.IN_OPEN
	WatchMovedFrom              = C.IN_MOVED_FROM
	WatchMovedTo                = C.IN_MOVED_TO
	WatchCreate                 = C.IN_CREATE
	WatchDelete                 = C.IN_DELETE
	WatchDeleteSelf             = C.IN_DELETE_SELF
	WatchMoveSelf               = C.IN_MOVE_SELF
	WatchOnlyDir                = C.IN_ONLYDIR
	WatchDontFollow             = C.IN_DONT_FOLLOW
	WatchExclUnlink             = C.IN_EXCL_UNLINK
	WatchMaskAdd                = C.IN_MASK_ADD
	WatchOneShot                = C.IN_ONESHOT
)

var notifier int

type WatchEvent struct {
	Path  string
	Mask  WatchMask
	Name  string
	Error error
}

func init() {
	notifier, _ = syscall.InotifyInit()
}

func Watch(path string, events WatchMask, queue chan<- WatchEvent) error {
	watchfd, err := syscall.InotifyAddWatch(notifier, path, uint32(events))

	if err != nil {
		return err
	}

	go func() {
		size := unsafe.Sizeof(C.struct_inotify_event{})
		buf := make([]byte, (size+C.FILENAME_MAX)*256)

		f := os.NewFile(uintptr(watchfd), path)

		for {
			var err error

			n, err := f.Read(buf)

			if err != nil {
				queue <- WatchEvent{
					Path:  path,
					Error: err,
				}

				break
			}

			reader := bytes.NewReader(buf[0:n])
			var ev C.struct_inotify_event

			for {
				err = binary.Read(reader, binary.LittleEndian, &ev)

				if err != nil {
					break
				}

				name := make([]byte, ev.len)
				_, err = reader.Read(name)

				if err != nil {
					break
				}

				queue <- WatchEvent{
					Path:  path,
					Mask:  WatchMask(ev.mask),
					Name:  string(name),
					Error: nil,
				}
			}

			if err != nil {
				queue <- WatchEvent{
					Path:  path,
					Error: err,
				}

				break
			}
		}
	}()

	return nil
}

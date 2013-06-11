package main

import (
	"io"
	"io/ioutil"
	"os"
	"path"
)

type DaemonCommands struct {
}

type Stage struct {
	Filename string
	Data     []byte

	Uid uint32
}

type StageReply struct {
	Info PackageInfo
}

type Incoming struct {
	Uid uint32
}

type PackageIds struct {
	Packages []uint64
	Uid      uint32
}

type PackageIdsReply struct {
	Packages []uint64
}

type Release PackageIds
type Discard PackageIds

type ReleaseReply PackageIdsReply
type DiscardReply PackageIdsReply

type GeneralReply struct {
}

type IncomingPackage struct {
	Name         string
	Id           uint64
	Distribution Distribution
	Files        []string
}

type IncomingReply struct {
	Packages []IncomingPackage
}

type WebQueueService struct {
	Uid uint32
}

type WebQueueReply struct {
	SocketFile string
	Uid        uint32
}

type WebQueueServer struct {
	SocketFile string
	Closer     chan bool
	Uid        uint32
}

var webqueue = map[string]*WebQueueServer{}

func (x *DaemonCommands) WebQueueService(service *WebQueueService, reply *WebQueueReply) error {
	f, err := ioutil.TempFile("", "autobuild-webqueue")

	if err != nil {
		return err
	}

	filename := f.Name()
	f.Close()
	os.Remove(filename)

	reply.SocketFile = filename

	closer, err := RunWebQueueService(filename, service.Uid)

	if err != nil {
		return err
	}

	webqueue[filename] = &WebQueueServer{
		SocketFile: filename,
		Closer:     closer,
		Uid:        service.Uid,
	}

	return nil
}

func (x *DaemonCommands) CloseWebQueueService(reply *WebQueueReply, g *GeneralReply) error {
	v := webqueue[reply.SocketFile]

	if v != nil && reply.Uid == v.Uid {
		v.Closer <- true
	}

	return nil
}

func (x *IncomingPackage) Matches(info *DistroBuildInfo) bool {
	return x.Distribution.Os == info.Distribution.Os &&
		x.Distribution.CodeName == info.Distribution.CodeName &&
		x.Distribution.Architectures[0] == info.Distribution.Architectures[0] &&
		x.Name == path.Base(info.Changes)
}

func (x *DaemonCommands) Stage(stage *Stage, reply *StageReply) error {
	info, err := builder.Stage(path.Base(stage.Filename),
		stage.Uid,
		func(b *PackageBuilder, writer io.Writer) error {
			_, err := writer.Write(stage.Data)
			return err
		})

	if err != nil {
		return err
	}

	reply.Info = *info
	return nil
}

func (x *DaemonCommands) makeIncomingPackage(d *DistroBuildInfo) IncomingPackage {
	ret := make([]string, len(d.ChangesFiles))

	for i, f := range d.ChangesFiles {
		ret[i] = f[len(options.Base)+1:]
	}

	return IncomingPackage{
		Name:         path.Base(d.Changes),
		Files:        ret,
		Distribution: d.Distribution,
		Id:           d.Id,
	}
}

func (x *DaemonCommands) Incoming(incoming *Incoming, reply *IncomingReply) error {
	return builder.Do(func(b *PackageBuilder) error {
		for _, res := range b.FinishedPackages {
			if res.Info.Uid != incoming.Uid {
				continue
			}

			for _, v := range res.Packages {
				p := x.makeIncomingPackage(v)
				reply.Packages = append(reply.Packages, p)
			}
		}

		return nil
	})
}

func (x *DaemonCommands) Release(release *Release, reply *ReleaseReply) error {
	pkgs, err := builder.Release(release.Packages, release.Uid)

	if err != nil {
		return err
	}

	reply.Packages = pkgs
	return nil
}

func (x *DaemonCommands) Discard(discard *Discard, reply *DiscardReply) error {
	pkgs, err := builder.Discard(discard.Packages, discard.Uid)

	if err != nil {
		return err
	}

	reply.Packages = pkgs
	return nil
}

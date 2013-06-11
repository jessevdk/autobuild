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
	Key          string
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

func (x *DaemonCommands) Stage(archive *Archive, reply *GeneralReply) error {
	return builder.Stage(path.Base(archive.Filename), func(b *PackageBuilder) (*PackageInfo, error) {
		// Create stage dir if necessary
		stagedir := path.Join(options.Base, "stage")
		os.MkdirAll(stagedir, 0755)

		// Write stage file data there
		basename := path.Base(archive.Filename)
		full := path.Join(stagedir, basename)

		f, err := os.Create(full)

		if err != nil {
			return nil, err
		}

		_, err = f.Write(archive.Data)

		f.Close()

		if err != nil {
			os.Remove(full)
			return nil, err
		}

		if err != nil {
			return nil, err
		}

		return NewPackageInfo(full, archive.Uid), nil
	})
}

func (x *DaemonCommands) makeIncomingPackage(key string, d *DistroBuildInfo) IncomingPackage {
	ret := make([]string, len(d.ChangesFiles))

	for i, f := range d.ChangesFiles {
		ret[i] = f[len(options.Base)+1:]
	}

	return IncomingPackage{
		Name:         path.Base(d.Changes),
		Files:        ret,
		Distribution: d.Distribution,
		Key:          key,
	}
}

func (x *DaemonCommands) Incoming(incoming *Incoming, reply *IncomingReply) error {
	return builder.Do(func(b *PackageBuilder) error {
		for _, res := range b.FinishedPackages {
			if res.Info.Uid != incoming.Uid {
				continue
			}

			for k, v := range res.Source {
				p := x.makeIncomingPackage(k, v)
				reply.Packages = append(reply.Packages, p)
			}

			for k, v := range res.Binaries {
				p := x.makeIncomingPackage(k, v)
				reply.Packages = append(reply.Packages, p)
			}
		}

		return nil
	})
}

func (x *DaemonCommands) doRelease(info *DistroBuildInfo) error {
	incomingdir := path.Join(options.Base,
		"repository",
		info.Distribution.Os,
		"incoming",
		info.Distribution.CodeName)

	os.MkdirAll(incomingdir, 0755)

	// To release, we move all the registered files to the reprepro
	// incoming
	for _, f := range info.Files {
		target := path.Join(incomingdir, path.Base(f))

		if err := os.Rename(f, target); err != nil {
			// Try copy instead
			fr, err := os.Open(f)

			if err != nil {
				return err
			}

			defer fr.Close()
			os.MkdirAll(path.Dir(target), 0755)

			fw, err := os.Create(target)

			if err != nil {
				return err
			}

			defer fw.Close()

			if _, err := io.Copy(fw, fr); err != nil {
				return err
			}
		}
	}

	return nil
}

func (x *DaemonCommands) Release(release *Release, reply *GeneralReply) error {
	return builder.Do(func(b *PackageBuilder) error {
		runReproMutex.Lock()

		distros := make(map[string]Distribution)

		resultscp := b.FinishedPackages
		b.FinishedPackages = make([]*BuildInfo, 0, len(resultscp))
		var err error

		for _, res := range resultscp {
			if res.Info.Uid != release.Uid {
				b.FinishedPackages = append(b.FinishedPackages, res)
				continue
			}

			for _, f := range release.Packages {
				if v, ok := res.Source[f.Key]; ok && f.Matches(v) {
					if err = x.doRelease(v); err != nil {
						break
					}

					distros[v.Distribution.SourceName()] = v.Distribution

					delete(res.Source, f.Key)
				} else if v, ok := res.Binaries[f.Key]; ok && f.Matches(v) {
					if err = x.doRelease(v); err != nil {
						break
					}

					distros[v.Distribution.SourceName()] = v.Distribution
					delete(res.Binaries, f.Key)
				}
			}

			if len(res.Source) != 0 || len(res.Binaries) != 0 {
				b.FinishedPackages = append(b.FinishedPackages, res)
			}

			if err != nil {
				break
			}
		}

		runReproMutex.Unlock()

		for _, v := range distros {
			runRepRepro(&v)
		}

		return err
	})
}

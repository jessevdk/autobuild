package main

import (
	"bufio"
	"bytes"
	"compress/gzip"
	"encoding/gob"
	"encoding/json"
	"fmt"
	"io"
	"io/ioutil"
	"os"
	"os/user"
	"path"
	"regexp"
	"sort"
	"strconv"
	"strings"
	"sync"
	"sync/atomic"
)

type Error string

func (x *Error) MarshalJSON() ([]byte, error) {
	if x == nil {
		return nil, nil
	}

	return json.Marshal(x.Error())
}

func (x Error) Error() string {
	return string(x)
}

func WrapError(err error) error {
	if err == nil {
		return nil
	}

	if _, ok := err.(Error); ok {
		return err
	} else {
		return Error(err.Error())
	}

}

type DistroBuildInfo struct {
	IncomingDir  string
	Changes      string
	Distribution Distribution
	ChangesFiles []string
	Files        []string
	Error        error
	Log          string `json:"-"`
	Id           uint64
}

type DistroBuildInfoMap map[uint64]*DistroBuildInfo

type BuildInfo struct {
	Info    *PackageInfo
	Package *ExtractedPackage

	BuildResultsDir string
	Error           error

	Source   map[string]*DistroBuildInfo
	Binaries map[string]*DistroBuildInfo
	Packages DistroBuildInfoMap
}

func (x DistroBuildInfoMap) MarshalJSON() ([]byte, error) {
	rm := make(map[string]*DistroBuildInfo)

	for k, v := range x {
		rm[fmt.Sprintf("%v", k)] = v
	}

	return json.Marshal(rm)
}

func (x DistroBuildInfoMap) UnmarshalJSON(data []byte) error {
	rm := make(map[string]*DistroBuildInfo)

	if err := json.Unmarshal(data, rm); err != nil {
		return err
	}

	for k, v := range rm {
		ki, err := strconv.ParseUint(k, 10, 64)

		if err != nil {
			return err
		}

		x[ki] = v
	}

	return nil
}

type ExtractedPackage struct {
	Dir     string
	OrigGz  string
	DiffGz  string
	Patches map[string]string
	Options BuildOptions
}

type PackageBuilder struct {
	CurrentlyBuilding *PackageInfo
	FinishedPackages  []*BuildInfo
	PackageQueue      []*PackageInfo

	BuildInfoMap map[uint64]*BuildInfo

	notifyQueue chan bool

	Mutex     sync.Mutex
	PackageId uint64
}

var builder = PackageBuilder{
	BuildInfoMap: make(map[uint64]*BuildInfo),
	notifyQueue:  make(chan bool, 1024),
}

var packageInfoRegex *regexp.Regexp
var changelogSubstituteRegex *regexp.Regexp

func (x *PackageBuilder) Do(fn func(b *PackageBuilder) error) error {
	x.Mutex.Lock()
	err := fn(x)
	x.Mutex.Unlock()

	return err
}

func (x *PackageBuilder) Stage(pname string,
	uid uint32,
	fn func(x *PackageBuilder, writer io.Writer) error) (*PackageInfo, error) {
	var info *PackageInfo

	return info, x.Do(func(b *PackageBuilder) error {
		// Check if we are currently building this package
		if b.CurrentlyBuilding.MatchStageFile(pname) {
			return fmt.Errorf("The file `%s' is currently building. Please wait until the built is finished to build the package again.", pname)
		}

		// Check results
		for _, binfo := range b.FinishedPackages {
			if binfo.Info.MatchStageFile(pname) {
				return fmt.Errorf("The file `%s' has already been built and is waiting to be released. Use `autobuild release' to release or discard before building again.", pname)
			}
		}

		// Check queue
		for _, info := range b.PackageQueue {
			if info.MatchStageFile(pname) {
				return fmt.Errorf("The file `%s' has already been queued to be built. Use `autobuild queue' to remove the queued package first.", pname)
			}
		}

		stagedir := path.Join(options.Base, "stage")
		os.MkdirAll(stagedir, 0755)

		stagefile := path.Join(stagedir, pname)

		f, err := os.Create(stagefile)

		if err != nil {
			return err
		}

		if err := fn(b, f); err != nil {
			f.Close()
			os.Remove(stagefile)

			return err
		}

		f.Close()

		info = NewPackageInfo(stagefile, uid)

		b.PackageQueue = append(b.PackageQueue, info)
		b.notifyQueue <- true

		return nil
	})
}

func (x *PackageBuilder) Run() {
	for {
		select {
		case _ = <-x.notifyQueue:
			if x.CurrentlyBuilding == nil {
				x.Do(func(b *PackageBuilder) error {
					if len(b.PackageQueue) > 0 {
						b.CurrentlyBuilding = b.PackageQueue[0]
						b.PackageQueue = b.PackageQueue[1:]
					}

					return nil
				})
			}

			if x.CurrentlyBuilding != nil {
				binfo := x.buildPackage()

				if options.Verbose {
					fmt.Printf("Finished building `%s'\n", path.Base(binfo.Info.StageFile))
				}

				x.Do(func(b *PackageBuilder) error {
					b.FinishedPackages = append(b.FinishedPackages, binfo)

					for _, p := range binfo.Packages {
						b.BuildInfoMap[p.Id] = binfo
					}

					b.CurrentlyBuilding = nil

					if len(b.PackageQueue) > 0 {
						b.notifyQueue <- true
					}

					return nil
				})
			}
		}
	}
}

func (x *PackageBuilder) extractPackage(info *PackageInfo) (*ExtractedPackage, error) {
	if options.Verbose {
		fmt.Printf("Extracting package `%s'...\n", info.Name)
	}

	defer os.Remove(info.StageFile)

	tmp := path.Join(options.Base, "tmp")
	os.MkdirAll(tmp, 0755)

	tdir, err := ioutil.TempDir(tmp, "autobuild")

	if err != nil {
		return nil, fmt.Errorf("Failed to create temporary directory to extract package: %s", err)
	}

	z := "gz"

	switch info.Compression {
	case "xz":
		z = "J"
	case "bz2":
		z = "j"
	}

	// Extract archive
	if err := RunCommandIn(tdir, "tar", "-x"+z+"f", info.StageFile); err != nil {
		os.RemoveAll(tdir)
		return nil, fmt.Errorf("Failed to extract staged package `%s': %s", path.Base(info.StageFile), err)
	}

	// Look for options
	bopts := options.BuildOptions

	f, err := os.Open(path.Join(tdir, "options"))

	if err == nil {
		if options.Verbose {
			fmt.Printf("Parsing package options...\n")
		}

		dec := json.NewDecoder(f)
		dec.Decode(&bopts)
		f.Close()
	}

	if options.Verbose {
		fmt.Printf("Checking for %s_%s.orig.tar.gz...\n", info.Name, info.Version)
	}

	origgz := path.Join(tdir, fmt.Sprintf("%s_%s.orig.tar.gz", info.Name, info.Version))

	if _, err := os.Stat(origgz); err != nil {
		os.RemoveAll(tdir)

		return nil, fmt.Errorf("The stage file `%s' does not contain the original tarball `%s'",
			path.Base(info.StageFile), path.Base(origgz))
	}

	if options.Verbose {
		fmt.Printf("Checking for %s_%s.diff.gz...\n", info.Name, info.Version)
	}

	diffgz := path.Join(tdir, fmt.Sprintf("%s_%s.diff.gz", info.Name, info.Version))

	if _, err := os.Stat(origgz); err != nil {
		os.RemoveAll(tdir)

		return nil, fmt.Errorf("The stage file `%s' does not contain the debian diff `%s'",
			path.Base(info.StageFile), path.Base(diffgz))
	}

	if options.Verbose {
		fmt.Printf("Extracting additional patches...\n")
	}

	// Extract patches
	patchdir := path.Join(tdir, "patches")
	patches := make(map[string]string, 0)

	if f, err := os.Open(patchdir); err == nil {
		names, _ := f.Readdirnames(0)

		for _, name := range names {
			fullname := path.Join(patchdir, name)
			ext := path.Ext(name)
			name = name[0 : len(name)-len(ext)]

			switch ext {
			case ".xz":
				RunCommandIn(tdir, "unxz", fullname)
			case ".gz":
				RunCommandIn(tdir, "gunzip", fullname)
			case ".bz2":
				RunCommandIn(tdir, "bunzip2", fullname)
			default:
			}

			patches[path.Base(name)] = path.Join(patchdir, name)
		}
	}

	return &ExtractedPackage{
		Dir:     tdir,
		OrigGz:  origgz,
		DiffGz:  diffgz,
		Patches: patches,
		Options: bopts,
	}, nil
}

func (x *PackageBuilder) substituteUnreleased(changelog string, distro *Distribution) error {
	// Read complete file
	b, err := ioutil.ReadFile(changelog)

	if err != nil {
		return fmt.Errorf("Failed to open debian/changelog for substitution: %s", err)
	}

	repl := fmt.Sprintf("-${1}%s0) %s", distro.CodeName, distro.CodeName)

	// Substitute
	ret := changelogSubstituteRegex.ReplaceAllString(string(b), repl)

	// Write changelog back
	if err := ioutil.WriteFile(changelog, []byte(ret), 0644); err != nil {
		return fmt.Errorf("Failed to write debian/changelog for substitution: %s", err)
	}

	return nil
}

func (x *PackageBuilder) readLines(rd *bufio.Reader, fn func(line string) error) error {
	for {
		line, err := rd.ReadString('\n')

		if err != nil && err != io.EOF {
			return err
		}

		if err == nil {
			line = line[0 : len(line)-1]
		}

		e := fn(line)

		if e != nil {
			return e
		} else if err != nil {
			return err
		}
	}

	return nil
}

func (x *PackageBuilder) parseChanges(info *DistroBuildInfo) error {
	f, err := os.Open(info.Changes + ".changes")

	if err != nil {
		return err
	}

	defer f.Close()

	rd := bufio.NewReader(f)

	err = x.readLines(rd, func(line string) error {
		line = strings.TrimSpace(line)

		if line == "Files:" {
			return io.EOF
		}

		return nil
	})

	if err != nil && err != io.EOF {
		return err
	}

	info.ChangesFiles = make([]string, 0)

	err = x.readLines(rd, func(line string) error {
		parts := strings.SplitN(strings.TrimSpace(line), " ", 5)

		if len(parts) == 5 {
			file := path.Join(info.IncomingDir, parts[4])
			info.ChangesFiles = append(info.ChangesFiles, file)
		}

		return nil
	})

	if err != nil && err != io.EOF {
		return err
	}

	return nil
}

func (x *PackageBuilder) chownToUser(target string, binfo *BuildInfo) error {
	var gid uint32

	if len(options.Group) != 0 && options.GroupId != 0 {
		gid = options.GroupId
	} else {
		us, _ := user.LookupId(fmt.Sprintf("%v", binfo.Info.Uid))

		if us != nil {
			g, err := strconv.ParseUint(us.Gid, 10, 32)

			if err == nil {
				gid = uint32(g)
			}
		}
	}

	return os.Chown(target, int(binfo.Info.Uid), int(gid))
}

func (x *PackageBuilder) moveResults(binfo *BuildInfo, info *DistroBuildInfo, resdir string, rmfiles ...string) error {
	incoming := info.IncomingDir
	os.MkdirAll(incoming, 0755)

	ret := make([]string, 0)

	d, err := os.Open(resdir)

	if err != nil {
		return fmt.Errorf("Failed to open results directory `%s': %s",
			resdir,
			err)
	}

	names, err := d.Readdirnames(0)

	if err != nil {
		return fmt.Errorf("Failed to read build results from `%s': %s",
			resdir,
			err)
	}

	rmmapping := make(map[string]bool)

	for _, rm := range rmfiles {
		rmmapping[rm] = true
	}

	chsuf := ".changes"

	for _, name := range names {
		filename := path.Join(resdir, name)
		target := path.Join(incoming, name)

		// Remove filename if the target was already built
		if rmmapping[target] {
			os.Remove(filename)
		} else {
			if err := MoveFile(filename, target); err != nil {
				return fmt.Errorf("Failed to move build result `%s' to target location `%s': %s",
					filename,
					incoming,
					err)
			}

			x.chownToUser(target, binfo)

			if strings.HasSuffix(target, chsuf) {
				info.Changes = target[0 : len(target)-len(chsuf)]

				if err := x.parseChanges(info); err != nil {
					return fmt.Errorf("Failed to parse changes file `%s': %s",
						info.Changes+".changes",
						err)
				}
			}

			ret = append(ret, target)
		}
	}

	info.Files = ret
	return nil
}

func (x *PackageBuilder) extractSourcePackage(info *BuildInfo, distro *Distribution) error {
	pack := info.Package

	pkgdir := path.Join(pack.Dir, fmt.Sprintf("%s-%s", info.Info.Name, info.Info.Version))
	os.RemoveAll(pkgdir)

	if options.Verbose {
		fmt.Printf("Extracting: %v...\n", pack.OrigGz)
	}

	// Extract original orig.tar.gz
	if err := RunCommandIn(pack.Dir, "tar", "-xzf", pack.OrigGz); err != nil {
		return fmt.Errorf("Failed to extract original tarball `%s': %s",
			path.Base(pack.OrigGz), err)
	}

	// Check if it was extracted correctly
	if _, err := os.Stat(pkgdir); err != nil {
		return fmt.Errorf("Did not find original source `%s' after extract original tarball `%s'",
			path.Base(pkgdir),
			path.Base(pack.OrigGz))
	}

	if options.Verbose {
		fmt.Printf("Extracting debian diff: %v...\n", pack.DiffGz)
	}

	// Apply the diff.gz debian patch
	dgz, err := os.Open(pack.DiffGz)

	if err != nil {
		return fmt.Errorf("Failed to open debian diff `%s': %s",
			path.Base(pack.DiffGz), err)
	}

	defer dgz.Close()

	rd, err := gzip.NewReader(dgz)

	if err != nil {
		return fmt.Errorf("Failed to extract debian diff `%s': %s",
			path.Base(pack.DiffGz), err)
	}

	if options.Verbose {
		fmt.Printf("Patching...\n")
	}

	cmd := MakeCommandIn(pkgdir, "patch", "-p1")
	cmd.Stdin = rd

	cmd.Stdout = nil
	cmd.Stderr = nil

	if out, err := cmd.CombinedOutput(); err != nil {
		return fmt.Errorf("Failed to run debian debian patch: %s",
			string(out))
	}

	dgz.Close()

	if _, err := os.Stat(path.Join(pkgdir, "debian")); err != nil {
		return fmt.Errorf("Could not find `debian' directory after applying debian patch")
	}

	// Apply distribution specific patches
	if patch, ok := pack.Patches[distro.CodeName]; ok {
		if options.Verbose {
			fmt.Printf("Apply distribution specific patch: %v...\n", patch)
		}

		cmd := MakeCommandIn(pkgdir, "patch", "-p1", "-i", patch)
		cmd.Stdout = nil
		cmd.Stderr = nil

		if out, err := cmd.CombinedOutput(); err != nil {
			return fmt.Errorf("Failed to apply distribution specific patch `%s': %s",
				path.Base(patch),
				string(out))
		}
	}

	if options.Verbose {
		fmt.Printf("Substitute ChangeLog...\n")
	}

	// Replace UNRELEASED in changelog with specific distro
	changelog := path.Join(pkgdir, "debian", "changelog")

	if err := x.substituteUnreleased(changelog, distro); err != nil {
		return err
	}

	return nil
}

func (x *PackageBuilder) buildSourcePackage(info *BuildInfo, distro *Distribution) *DistroBuildInfo {
	src := &DistroBuildInfo{
		IncomingDir: path.Join(options.Base, "incoming", distro.Os, distro.CodeName),

		Distribution: Distribution{
			Os:            distro.Os,
			CodeName:      distro.CodeName,
			Architectures: []string{"source"},
		},

		Id: atomic.AddUint64(&x.PackageId, 1),
	}

	if options.Verbose {
		fmt.Printf("Building source package...\n")
	}

	src.Error = WrapError(x.extractSourcePackage(info, distro))

	if src.Error != nil {
		info.Packages[src.Id] = src
		return src
	}

	pkgdir := path.Join(info.Package.Dir, fmt.Sprintf("%s-%s", info.Info.Name, info.Info.Version))

	// Call pdebuild
	cmd := MakeCommandIn(pkgdir,
		"pdebuild",
		"--pbuilder", options.Pbuilder,
		"--configfile", path.Join(options.Base, "etc", "pbuilderrc"),
		"--buildresult", info.BuildResultsDir,
		"--debbuildopts", "-us",
		"--debbuildopts", "-uc",
		"--debbuildopts", "-S")

	cmd.Env = os.Environ()
	cmd.Env = append(cmd.Env, fmt.Sprintf("DIST=%s/%s", distro.Os, distro.CodeName))
	cmd.Env = append(cmd.Env, fmt.Sprintf("AUTOBUILD_BASE=%s", options.Base))

	var wr io.Writer
	log := &bytes.Buffer{}

	if options.Verbose {
		wr = io.MultiWriter(log, os.Stdout)
	} else {
		wr = log
	}

	cmd.Stdout = wr
	cmd.Stderr = wr

	if options.Verbose {
		fmt.Printf("Run pdebuild for source in `%s'...\n", info.Package.Dir)
	}

	src.Error = WrapError(cmd.Run())

	src.Log = log.String()

	if src.Error != nil {
		os.RemoveAll(info.BuildResultsDir)
	} else {
		// Move build results to incoming
		x.moveResults(info, src, info.BuildResultsDir)
	}

	info.Packages[src.Id] = src
	return src
}

func (x *PackageBuilder) buildBinaryPackages(info *BuildInfo, src *DistroBuildInfo, distro *Distribution, arch string, buildBinaryIndep bool) error {
	bin := &DistroBuildInfo{
		IncomingDir: path.Join(options.Base, "incoming", distro.Os, distro.CodeName),

		Distribution: Distribution{
			Os:            distro.Os,
			CodeName:      distro.CodeName,
			Architectures: []string{arch},
		},

		Id: atomic.AddUint64(&x.PackageId, 1),
	}

	var debBuildOpt string

	if buildBinaryIndep == true {
		debBuildOpt = "-b"
	} else {
		// from 'man depkg-buildpackage' -B : build binary package, limited to binary dependent
		debBuildOpt = "-B"
	}

	pkgdir := path.Join(info.Package.Dir, fmt.Sprintf("%s-%s", info.Info.Name, info.Info.Version))

	// Call pdebuild
	cmd := MakeCommandIn(pkgdir,
		"pdebuild",
		"--pbuilder", options.Pbuilder,
		"--configfile", path.Join(options.Base, "etc", "pbuilderrc"),
		"--buildresult", info.BuildResultsDir,
		"--debbuildopts", "-us",
		"--debbuildopts", "-uc",
		"--debbuildopts", debBuildOpt)

	cmd.Env = os.Environ()
	cmd.Env = append(cmd.Env, fmt.Sprintf("DIST=%s/%s", distro.Os, distro.CodeName))
	cmd.Env = append(cmd.Env, fmt.Sprintf("ARCH=%s", arch))
	cmd.Env = append(cmd.Env, fmt.Sprintf("AUTOBUILD_BASE=%s", options.Base))

	var wr io.Writer
	log := &bytes.Buffer{}

	if options.Verbose {
		wr = io.MultiWriter(log, os.Stdout)
	} else {
		wr = log
	}

	cmd.Stdout = wr
	cmd.Stderr = wr

	bin.Error = WrapError(cmd.Run())

	bin.Log = log.String()

	if bin.Error != nil {
		os.RemoveAll(info.BuildResultsDir)
	} else {
		// Move build results to incoming (skipping source files)
		x.moveResults(info, bin, info.BuildResultsDir, src.Files...)
	}

	info.Packages[bin.Id] = bin
	return bin.Error
}

func (x *PackageBuilder) buildPackage() *BuildInfo {
	info := x.CurrentlyBuilding

	if options.Verbose {
		fmt.Printf("Building package %v (%v): %v\n", info.StageFile, info.Name, info.Version)
	}

	binfo := &BuildInfo{
		Info:     info,
		Packages: make(map[uint64]*DistroBuildInfo),
	}

	pack, err := x.extractPackage(info)

	if err != nil {
		binfo.Error = WrapError(err)
		return binfo
	}

	buildresult := path.Join(pack.Dir, "result")

	os.RemoveAll(buildresult)
	os.MkdirAll(buildresult, 0755)

	binfo.BuildResultsDir = buildresult
	binfo.Package = pack

	defer os.RemoveAll(pack.Dir)

	// For each distribution
	for _, distro := range pack.Options.Distributions {
		src := x.buildSourcePackage(binfo, distro)

		if src.Error != nil {
			return binfo
		}

		for i, arch := range distro.Architectures {
			//we build binary-indep packages only for the first architecture supported
			buildBinaryIndep := i == 0

			x.buildBinaryPackages(binfo, src, distro, arch, buildBinaryIndep)
		}
	}

	return binfo
}

type Uint64Slice []uint64

func (p Uint64Slice) Len() int {
	return len(p)
}

func (p Uint64Slice) Less(i int, j int) bool {
	return p[i] < p[j]
}

func (p Uint64Slice) Search(x uint64) int {
	return sort.Search(len(p), func(i int) bool {
		return p[i] >= x
	})
}

func (p Uint64Slice) Sort() {
	sort.Sort(p)
}

func (p Uint64Slice) Swap(i int, j int) {
	p[i], p[j] = p[j], p[i]
}

func (p Uint64Slice) Contains(x uint64) bool {
	i := p.Search(x)

	return i < len(p) && p[i] == x
}

func (x *PackageBuilder) doRelease(info *DistroBuildInfo) error {
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

		if err := MoveFile(f, target); err != nil {
			return err
		}
	}

	return nil
}

func (x *PackageBuilder) doDiscard(info *DistroBuildInfo) error {
	// To discard, we simply remove the files
	for _, f := range info.Files {
		os.Remove(f)
	}

	return nil
}

func (x *PackageBuilder) removeFinished() {
	finishedp := make([]*BuildInfo, 0, len(x.FinishedPackages))

	for _, res := range x.FinishedPackages {
		if len(res.Packages) != 0 {
			finishedp = append(finishedp, res)
		}
	}

	x.FinishedPackages = finishedp
}

func (x *PackageBuilder) filterOwned(ids []uint64, uid uint32) []uint64 {
	ret := make([]uint64, 0, len(ids))

	for _, id := range ids {
		binfo := x.BuildInfoMap[id]

		if binfo != nil && binfo.Info.Uid == uid {
			ret = append(ret, id)
		}
	}

	return ret
}

func (x *PackageBuilder) Discard(ids []uint64, uid uint32) ([]uint64, error) {
	retval := make([]uint64, 0, len(ids))

	return retval, x.Do(func(b *PackageBuilder) error {
		ids = x.filterOwned(ids, uid)

		err := x.foreachMatchedId(ids, func(info *BuildInfo, binfo *DistroBuildInfo) error {
			if err := x.doDiscard(binfo); err != nil {
				return err
			}

			retval = append(retval, binfo.Id)
			return nil
		})

		x.removeFinished()
		return err
	})
}

func (x *PackageBuilder) Release(ids []uint64, uid uint32) ([]uint64, error) {
	retval := make([]uint64, 0, len(ids))

	return retval, x.Do(func(b *PackageBuilder) error {
		runReproMutex.Lock()

		distros := make(map[string]Distribution)

		ids = x.filterOwned(ids, uid)

		err := x.foreachMatchedId(ids, func(info *BuildInfo, binfo *DistroBuildInfo) error {
			if err := x.doRelease(binfo); err != nil {
				return err
			}

			distros[binfo.Distribution.SourceName()] = binfo.Distribution
			retval = append(retval, binfo.Id)

			return nil
		})

		x.removeFinished()
		runReproMutex.Unlock()

		for _, v := range distros {
			runRepRepro(&v)
		}

		return err
	})
}

func (x *PackageBuilder) foreachMatchedId(ids []uint64, fn func(info *BuildInfo, binfo *DistroBuildInfo) error) error {
	sortedids := Uint64Slice(ids)
	sortedids.Sort()

	for _, res := range x.FinishedPackages {
		delmap := make([]uint64, 0, len(res.Packages))

		var err error

		for _, v := range res.Packages {
			if sortedids.Contains(v.Id) {
				if err = fn(res, v); err != nil {
					break
				}

				delmap = append(delmap, v.Id)
			}
		}

		for _, k := range delmap {
			delete(res.Packages, k)
			delete(x.BuildInfoMap, k)
		}

		if err != nil {
			return err
		}
	}

	return nil
}

type PackageBuilderState struct {
	FinishedPackages []*BuildInfo
	PackageQueue     []*PackageInfo
	PackageId        uint64
}

func (x *PackageBuilder) Save() error {
	f := path.Join(options.Base, "run", "builder.state")

	return x.Do(func(b *PackageBuilder) error {
		state := PackageBuilderState{
			FinishedPackages: b.FinishedPackages,
			PackageQueue:     b.PackageQueue,
			PackageId:        b.PackageId,
		}

		if b.CurrentlyBuilding != nil {
			state.PackageQueue = append([]*PackageInfo{b.CurrentlyBuilding}, state.PackageQueue...)
		}

		fn, err := os.Create(f)

		if err != nil {
			return err
		}

		enc := gob.NewEncoder(fn)

		if err := enc.Encode(state); err != nil {
			return err
		}

		fn.Close()
		return nil
	})
}

func (x *PackageBuilder) Load() error {
	f := path.Join(options.Base, "run", "builder.state")

	return x.Do(func(b *PackageBuilder) error {
		fn, err := os.Open(f)

		if err == nil {
			defer fn.Close()

			dec := gob.NewDecoder(fn)

			state := &PackageBuilderState{}
			err := dec.Decode(state)

			if err != nil {
				return err
			}

			b.FinishedPackages = state.FinishedPackages

			b.PackageQueue = state.PackageQueue
			b.PackageId = state.PackageId

			for _, info := range b.FinishedPackages {
				for _, binfo := range info.Packages {
					b.BuildInfoMap[binfo.Id] = info
				}
			}

			if len(b.PackageQueue) > 0 {
				b.notifyQueue <- true
			}

			return nil
		}

		return nil
	})
}

func (x *PackageBuilder) FindPackage(id uint64) (*BuildInfo, *DistroBuildInfo) {
	binfo := x.BuildInfoMap[id]

	if binfo == nil {
		return nil, nil
	}

	info := binfo.Packages[id]

	if info != nil {
		return binfo, info
	}

	return nil, nil
}

func init() {
	packageInfoRegex, _ = regexp.Compile(`(.*)[_-]([0-9]+(\.[0-9]+)+).tar.(gz|xz|bz2)`)
	changelogSubstituteRegex, _ = regexp.Compile(`-([0-9]+)\) UNRELEASED`)
}

package main

import (
	"compress/gzip"
	"fmt"
	"io/ioutil"
	"os"
	"path"
	"regexp"
	"io"
	"bufio"
	"strings"
	"sync"
	"encoding/gob"
	"encoding/json"
)

type DistroBuildInfo struct {
	IncomingDir string
	ResultsDir  string
	Changes     string
	Distribution Distribution
	ChangesFiles []string
	Files       []string
}

type BuildInfo struct {
	Info    *PackageInfo
	Package *ExtractedPackage

	ResultsDir      string
	BuildResultsDir string

	Source   map[string]*DistroBuildInfo
	Binaries map[string]*DistroBuildInfo

	Error    error
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
	FinishedPackages []*BuildInfo
	PackageQueue []*PackageInfo

	notifyQueue chan bool

	Mutex sync.Mutex
}

var builder PackageBuilder

var packageInfoRegex *regexp.Regexp
var changelogSubstituteRegex *regexp.Regexp

func (x *PackageBuilder) Do(fn func (b *PackageBuilder) error) error {
	x.Mutex.Lock()
	err := fn(x)
	x.Mutex.Unlock()

	return err
}

func (x *PackageBuilder) Stage(pname string, fn func (x *PackageBuilder) (*PackageInfo, error)) error {
	return x.Do(func (b *PackageBuilder) error {
		// Check if we are currently building this package
		if b.CurrentlyBuilding.MatchStageFile(pname) {
			return fmt.Errorf("The file `%s' is currently building. Please wait until the built is finished to build the package again.", pname)
		}

		// Check results
		for _, binfo := range b.FinishedPackages {
			if binfo.Info.MatchStageFile(pname) && binfo.Error == nil {
				return fmt.Errorf("The file `%s' has already been built and is waiting to be released. Use `autobuild release' to release or discard before building again.")
			}
		}

		// Check queue
		for _, info := range b.PackageQueue {
			if info.MatchStageFile(pname) {
				return fmt.Errorf("The file `%s' has already been queued to be built. Use `autobuild queue' to remove the queued package first.")
			}
		}

		info, err := fn(b)

		if err != nil {
			return err
		}

		b.PackageQueue = append(b.PackageQueue, info)
		b.notifyQueue <- true

		return nil
	})
}

func (x *PackageBuilder) Run() {
	for {
		select {
		case _ = <- x.notifyQueue:
			if x.CurrentlyBuilding == nil {
				x.Do(func (b *PackageBuilder) error {
					if len(b.PackageQueue) > 0 {
						b.CurrentlyBuilding = b.PackageQueue[0]
						b.PackageQueue = b.PackageQueue[1:]
					}

					return nil
				})
			}

			if x.CurrentlyBuilding != nil {
				binfo := x.buildPackage()

				x.Do(func (b *PackageBuilder) error {
					b.FinishedPackages = append(b.FinishedPackages, binfo)
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

	tmp := path.Join(options.Base, "tmp")
	os.MkdirAll(tmp, 0755)

	tdir, err := ioutil.TempDir(tmp, "autobuild")

	if err != nil {
		return nil, err
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
		return nil, err
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
		return nil, err
	}

	if options.Verbose {
		fmt.Printf("Checking for %s_%s.diff.gz...\n", info.Name, info.Version)
	}

	diffgz := path.Join(tdir, fmt.Sprintf("%s_%s.diff.gz", info.Name, info.Version))

	if _, err := os.Stat(origgz); err != nil {
		os.RemoveAll(tdir)
		return nil, err
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
		return err
	}

	repl := fmt.Sprintf("-${1}%s0) %s", distro.CodeName, distro.CodeName)

	// Substitute
	ret := changelogSubstituteRegex.ReplaceAllString(string(b), repl)

	// Write changelog back
	return ioutil.WriteFile(changelog, []byte(ret), 0644)
}

func (x *PackageBuilder) readLines(rd *bufio.Reader, fn func (line string) error) error {
	for {
		line, err := rd.ReadString('\n')

		if err != nil && err != io.EOF {
			return err
		}

		if err == nil {
			line = line[0:len(line)-1]
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

	err = x.readLines(rd, func (line string) error {
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

	err = x.readLines(rd, func (line string) error {
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

func (x *PackageBuilder) moveResults(info *DistroBuildInfo, resdir string, rmfiles ...string) error {
	incoming := info.IncomingDir
	os.MkdirAll(incoming, 0755)

	ret := make([]string, 0)

	d, err := os.Open(resdir)

	if err != nil {
		return err
	}

	names, err := d.Readdirnames(0)

	if err != nil {
		return err
	}

	rmmapping := make(map[string]struct{})

	for _, rm := range rmfiles {
		rmmapping[rm] = struct{}{}
	}

	chsuf := ".changes"

	for _, name := range names {
		filename := path.Join(resdir, name)
		target := path.Join(incoming, name)

		// Remove filename if the target was already built
		if _, ok := rmmapping[target]; ok {
			if err := os.Remove(filename); err != nil {
				return err
			}
		} else {
			if err := os.Rename(filename, target); err != nil {
				return err
			}

			if strings.HasSuffix(target, chsuf) {
				info.Changes = target[0:len(target)-len(chsuf)]

				if err := x.parseChanges(info); err != nil {
					return err
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
		return err
	}

	// Check if it was extracted correctly
	if _, err := os.Stat(pkgdir); err != nil {
		return err
	}

	if options.Verbose {
		fmt.Printf("Extracting debian diff: %v...\n", pack.DiffGz)
	}

	// Apply the diff.gz debian patch
	dgz, err := os.Open(pack.DiffGz)

	if err != nil {
		return err
	}

	defer dgz.Close()

	rd, err := gzip.NewReader(dgz)

	if err != nil {
		return err
	}

	if options.Verbose {
		fmt.Printf("Patching...\n")
	}

	cmd := MakeCommandIn(pkgdir, "patch", "-p1")
	cmd.Stdin = rd

	if err := cmd.Run(); err != nil {
		return err
	}

	dgz.Close()

	if _, err := os.Stat(path.Join(pkgdir, "debian")); err != nil {
		return err
	}

	// Apply distribution specific patches
	if patch, ok := pack.Patches[distro.CodeName]; ok {
		if options.Verbose {
			fmt.Printf("Apply distribution specific patch: %v...\n", patch)
		}

		if err := RunCommandIn(pkgdir, "patch", "-p1", "-i", patch); err != nil {
			return err
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

func (x *PackageBuilder) buildSourcePackage(info *BuildInfo, distro *Distribution) error {
	src := &DistroBuildInfo{
		ResultsDir:  path.Join(info.ResultsDir, fmt.Sprintf("%s-source", distro.CodeName)),
		IncomingDir: path.Join(options.Base, "incoming", distro.Os, distro.CodeName),

		Distribution: Distribution {
			Os: distro.Os,
			CodeName: distro.CodeName,
			Architectures: []string {"source"},
		},
	}

	if options.Verbose {
		fmt.Printf("Building source package...\n")
	}

	if err := x.extractSourcePackage(info, distro); err != nil {
		return err
	}

	// Make package local results dir
	os.MkdirAll(src.ResultsDir, 0755)

	llogpath := path.Join(src.ResultsDir, "log")
	llog, err := os.Create(llogpath)

	if err != nil {
		os.RemoveAll(src.ResultsDir)
		return err
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

	if options.Verbose {
		wr = io.MultiWriter(llog, os.Stdout)
	} else {
		wr = llog
	}

	cmd.Stdout = wr
	cmd.Stderr = wr

	if options.Verbose {
		fmt.Printf("Run pdebuild for source in `%s'...\n", info.Package.Dir)
	}

	if err := cmd.Run(); err != nil {
		llog.Close()

		// Still move the log
		os.RemoveAll(info.BuildResultsDir)
		return err
	}

	llog.Close()

	// Move build results to incoming
	x.moveResults(src, info.BuildResultsDir)

	info.Source[distro.SourceName()] = src
	return nil
}

func (x *PackageBuilder) buildBinaryPackages(info *BuildInfo, distro *Distribution, arch string) error {
	bin := &DistroBuildInfo{
		ResultsDir:  path.Join(info.ResultsDir, fmt.Sprintf("%s-%s", distro.CodeName, arch)),
		IncomingDir: path.Join(options.Base, "incoming", distro.Os, distro.CodeName),

		Distribution: Distribution {
			Os: distro.Os,
			CodeName: distro.CodeName,
			Architectures: []string {arch},
		},
	}

	// Make package local results dir
	os.MkdirAll(bin.ResultsDir, 0755)

	llogpath := path.Join(bin.ResultsDir, "log")
	llog, err := os.Create(llogpath)

	if err != nil {
		os.RemoveAll(bin.ResultsDir)
		return err
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
		"--debbuildopts", "-b")

	cmd.Env = os.Environ()
	cmd.Env = append(cmd.Env, fmt.Sprintf("DIST=%s/%s", distro.Os, distro.CodeName))
	cmd.Env = append(cmd.Env, fmt.Sprintf("ARCH=%s", arch))
	cmd.Env = append(cmd.Env, fmt.Sprintf("AUTOBUILD_BASE=%s", options.Base))

	var wr io.Writer

	if options.Verbose {
		wr = io.MultiWriter(llog, os.Stdout)
	} else {
		wr = llog
	}

	cmd.Stdout = wr
	cmd.Stderr = wr

	if err := cmd.Run(); err != nil {
		llog.Close()
		os.RemoveAll(info.BuildResultsDir)
		return err
	}

	llog.Close()

	// Move build results to incoming (skipping source files)
	x.moveResults(bin, info.BuildResultsDir, info.Source[distro.SourceName()].Files...)
	info.Binaries[distro.BinaryName(arch)] = bin

	return nil
}

func (x *PackageBuilder) buildPackage() *BuildInfo {
	info := x.CurrentlyBuilding

	if options.Verbose {
		fmt.Printf("Building package %v (%v): %v\n", info.StageFile, info.Name, info.Version)
	}

	binfo := &BuildInfo{
		Info:       info,
		ResultsDir: path.Join(options.Base, "results", fmt.Sprintf("%s-%s", info.Name, info.Version)),
		Source:     make(map[string]*DistroBuildInfo),
		Binaries:   make(map[string]*DistroBuildInfo),
		Error:      nil,
	}

	// Remove previous resdir if needed
	os.RemoveAll(binfo.ResultsDir)
	os.MkdirAll(binfo.ResultsDir, 0755)

	pack, err := x.extractPackage(info)

	if err != nil {
		binfo.Error = err
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
		if err := x.buildSourcePackage(binfo, distro); err != nil {
			binfo.Error = err
			return binfo
		}

		for _, arch := range distro.Architectures {
			if err := x.buildBinaryPackages(binfo, distro, arch); err != nil {
				binfo.Error = err
				return binfo
			}
		}
	}

	return binfo
}

type PackageBuilderState struct {
	FinishedPackages []*BuildInfo
	PackageQueue []*PackageInfo
}

func (x *PackageBuilder) Save() {
	f := path.Join(options.Base, "run", "builder.state")

	x.Do(func (b *PackageBuilder) error {
		state := PackageBuilderState {
			FinishedPackages: b.FinishedPackages,
			PackageQueue: b.PackageQueue,
		}

		if b.CurrentlyBuilding != nil {
			state.PackageQueue = append([]*PackageInfo {b.CurrentlyBuilding}, state.PackageQueue...)
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

func (x *PackageBuilder) Load() {
	f := path.Join(options.Base, "run", "builder.state")

	x.Do(func (b *PackageBuilder) error {
		fn, err := os.Open(f)

		if err != nil {
			defer fn.Close()

			dec := gob.NewDecoder(fn)

			state := &PackageBuilderState {}
			err := dec.Decode(state)

			if err != nil {
				return err
			}

			b.FinishedPackages = state.FinishedPackages
			b.PackageQueue = state.PackageQueue

			if len(b.PackageQueue) > 0 {
				b.notifyQueue <- true
			}

			return nil
		}

		return err
	})
}

func init() {
	packageInfoRegex, _ = regexp.Compile(`(.*)[_-]([0-9]+(\.[0-9]+)+).tar.(gz|xz|bz2)`)
	changelogSubstituteRegex, _ = regexp.Compile(`-([0-9]+)\) UNRELEASED`)
}

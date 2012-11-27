package main

import (
	"compress/gzip"
	"encoding/json"
	"errors"
	"fmt"
	"io/ioutil"
	"net"
	"net/rpc"
	"os"
	"os/signal"
	"path"
	"regexp"
	"syscall"
	"io"
)

type CommandDaemon struct {
}

type PackageInfo struct {
	StageFile   string
	Name        string
	Version     string
	Compression string
}

type DistroBuildInfo struct {
	IncomingDir string
	ResultsDir  string
	Files       []string
}

type BuildInfo struct {
	Info    *PackageInfo
	Package *ExtractedPackage

	ResultsDir      string
	BuildResultsDir string

	Source   map[string]*DistroBuildInfo
	Binaries map[string]*DistroBuildInfo
}

type Queue chan *PackageInfo

var queue Queue

var packageInfoRegex *regexp.Regexp
var changelogSubstituteRegex *regexp.Regexp

var results []*BuildInfo

func NewPackageInfo(filename string) *PackageInfo {
	basename := path.Base(filename)
	matched := packageInfoRegex.FindStringSubmatch(basename)

	if matched == nil {
		return nil
	}

	return &PackageInfo{
		StageFile:   filename,
		Name:        matched[1],
		Version:     matched[2],
		Compression: matched[4],
	}
}

func processStageEvent(ev WatchEvent) {
	filename := path.Join(ev.Path, ev.Name)

	// Added, stage it
	info := NewPackageInfo(filename)

	if info != nil {
		queue <- info
	} else {
		// Remove it
		os.Remove(filename)
	}
}

type ExtractedPackage struct {
	Dir     string
	OrigGz  string
	DiffGz  string
	Patches map[string]string
	Options BuildOptions
}

func extractPackage(info *PackageInfo) (*ExtractedPackage, error) {
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

func substituteUnreleased(changelog string, distro *Distribution) error {
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

func moveResults(info *DistroBuildInfo, resdir string, rmfiles ...string) error {
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

			ret = append(ret, target)
		}
	}

	info.Files = ret
	return nil
}

func extractSourcePackage(info *BuildInfo, distro *Distribution) error {
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

	if err := substituteUnreleased(changelog, distro); err != nil {
		return err
	}

	return nil
}

func buildSourcePackage(info *BuildInfo, distro *Distribution) error {
	src := &DistroBuildInfo{
		ResultsDir:  path.Join(info.ResultsDir, fmt.Sprintf("%s-source", distro.CodeName)),
		IncomingDir: path.Join(options.Base, "incoming", distro.Os, distro.CodeName),
	}

	if options.Verbose {
		fmt.Printf("Building source package...\n")
	}

	if err := extractSourcePackage(info, distro); err != nil {
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
	moveResults(src, info.BuildResultsDir)

	info.Source[distro.SourceName()] = src
	return nil
}

func buildBinaryPackages(info *BuildInfo, distro *Distribution, arch string) error {
	bin := &DistroBuildInfo{
		ResultsDir:  path.Join(info.ResultsDir, fmt.Sprintf("%s-%s", distro.CodeName, arch)),
		IncomingDir: path.Join(options.Base, "incoming", distro.Os, distro.CodeName),
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
	moveResults(bin, info.BuildResultsDir, info.Source[distro.SourceName()].Files...)
	info.Binaries[distro.BinaryName(arch)] = bin

	return nil
}

func buildPackage(info *PackageInfo) error {
	if options.Verbose {
		fmt.Printf("Building package %v (%v): %v\n", info.StageFile, info.Name, info.Version)
	}

	binfo := &BuildInfo{
		Info:       info,
		ResultsDir: path.Join(options.Base, "results", fmt.Sprintf("%s-%s", info.Name, info.Version)),
		Source:     make(map[string]*DistroBuildInfo),
		Binaries:   make(map[string]*DistroBuildInfo),
	}

	// Remove previous resdir if needed
	os.RemoveAll(binfo.ResultsDir)
	os.MkdirAll(binfo.ResultsDir, 0755)

	pack, err := extractPackage(info)

	if err != nil {
		return err
	}

	buildresult := path.Join(pack.Dir, "result")

	os.RemoveAll(buildresult)
	os.MkdirAll(buildresult, 0755)

	binfo.BuildResultsDir = buildresult
	binfo.Package = pack

	defer os.RemoveAll(pack.Dir)

	// For each distribution
	for _, distro := range pack.Options.Distributions {
		if err := buildSourcePackage(binfo, distro); err != nil {
			return err
		}

		for _, arch := range distro.Architectures {
			if err := buildBinaryPackages(binfo, distro, arch); err != nil {
				return err
			}
		}
	}

	results = append(results, binfo)
	return nil
}

func (x *CommandDaemon) listen() (*rpc.Server, error) {
	dirname := path.Join(options.Base, "run")
	os.MkdirAll(dirname, 0755)

	spath := path.Join(dirname, "autobuild.sock")
	listener, err := net.Listen("unix", spath)

	if err != nil {
		fmt.Errorf("Failed to create remote listen socket on `%s': %s\n",
		           spath,
		           err)

		return nil, err
	}

	os.Chmod(spath, 0777)

	cmds := &DaemonCommands{}

	server := rpc.NewServer()
	server.Register(cmds)

	go func() {
		ul := listener.(*net.UnixListener)

		for {
			cl, err := ul.AcceptUnix()

			if err != nil {
				break
			}

			if options.Verbose {
				fmt.Printf("New remote client connected...\n")
			}

			uid, _, err := RemoteRecvCredentials(cl)

			if err != nil {
				if options.Verbose {
					fmt.Printf("Failed to verify credentials: %s\n", err)
				}

				cl.Close()
				continue
			}

			if uid != options.UserId || options.UserId == 0 {
				if options.Verbose {
					fmt.Printf("User is not authenticated, closing connection...\n")
				}

				cl.Close()
			} else {
				go server.ServeConn(cl)
			}
		}
	}()

	return server, nil
}

func (x *CommandDaemon) Execute(args []string) error {
	syscall.RawSyscall(syscall.SYS_IOCTL, 0, uintptr(syscall.TIOCNOTTY), 0)

	// Run remote socket
	if _, err := x.listen(); err != nil {
		return err
	}

	sig := make(chan os.Signal, 10)
	signal.Notify(sig, syscall.SIGINT, syscall.SIGTERM)

	defer os.Remove(path.Join(options.Base, "run", "autobuild.sock"))

	for {
		select {
		case pack := <-queue:
			if err := buildPackage(pack); err != nil {
				if options.Verbose {
					fmt.Printf("Error during building of `%s': %s...\n",
					           pack.Name,
					           err)
				}
			}

			os.Remove(pack.StageFile)
		case s := <-sig:
			if s == syscall.SIGINT {
				return errors.New("")
			}

			return nil
		}
	}

	return nil
}

func init() {
	packageInfoRegex, _ = regexp.Compile(`(.*)[_-]([0-9]+(\.[0-9]+)+).tar.(gz|xz|bz2)`)
	changelogSubstituteRegex, _ = regexp.Compile(`-([0-9]+)\) UNRELEASED`)

	queue = make(Queue, 256)

	parser.AddCommand("Daemon", "daemon", &CommandDaemon{})
}

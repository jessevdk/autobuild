package main

import (
	"encoding/json"
	"fmt"
	"github.com/jessevdk/go-flags"
	"os"
	"path"
	"syscall"
)

// #include <sys/file.h>
import "C"

type BuildOptions struct {
	Distributions []*Distribution `json:"distributions,omit-empty"`
}

type RepositoryOptions struct {
	Origin      string `json:"origin,omit-empty" description:"The APT repository Origin field"`
	Label       string `json:"label,omit-empty" description:"The APT repository Label field"`
	Description string `json:"description,omit-empty" description:"The APT repository Description field"`
	SignKey     string `json:"sign-key,omit-empty" description:"The APT repository sign key identifier"`
	ListenPort  string `json:"listen-port,omit-empty" description:"The APT repository webserver port"`
}

type Options struct {
	Base     string                 `json:"base,omitempty"`
	BaseFlag func(val string) error `short:"b" long:"base" description:"Base autobuild directory" json:"-" default:"/var/lib/autobuild"`
	Verbose  bool                   `short:"v" long:"verbose" description:"Verbose output" json:"-"`
	Version  func() error           `short:"V" long:"version" description:"Print the version" json:"-"`

	Remote string `short:"r" long:"remote" json:"remote,omitempty" description:"Remote host for autobuild client commands"`

	BuildOptions BuildOptions           `json:"build-options,omit-empty" config:"-"`
	Pbuilder     string                 `json:"pbuilder"`
	UseTmpfs     bool                   `json:"use-tmpfs"`
	Repository   RepositoryOptions      `json:"repository"`
	GroupFlag    func(val string) error `short:"g" long:"group" description:"Authenticated group for autobuild communication" default:"autobuild" json:"-"`

	Group   string `json:"group,omitempty"`
	GroupId uint32 `json:"-"`
}

func (x *Options) LoadConfig() {
	// Load from json
	f, err := os.Open(path.Join(options.Base, "etc", "autobuild.json"))

	if err != nil {
		return
	}

	defer f.Close()

	dec := json.NewDecoder(f)
	dec.Decode(x)
}

func (x *BuildOptions) HasDistribution(distro *Distribution, arch string) bool {
	for _, distrocfg := range x.Distributions {
		if distrocfg.Os != distro.Os || distrocfg.CodeName != distro.CodeName {
			continue
		}

		for _, archcfg := range distrocfg.Architectures {
			if archcfg == arch {
				return true
			}
		}
	}

	return false
}

func (x *Options) UpdateConfig(updateFunc func(*Options) error) error {
	dirname := path.Join(options.Base, "etc")
	filename := path.Join(dirname, "autobuild.json")

	os.MkdirAll(dirname, 0755)

	f, err := os.OpenFile(filename, os.O_RDWR|os.O_CREATE, 0644)

	if err != nil {
		return err
	}

	defer f.Close()

	// Make sure we are the only one
	if err := syscall.Flock(int(f.Fd()), C.LOCK_EX); err != nil {
		return err
	}

	// Reload the config
	dec := json.NewDecoder(f)
	dec.Decode(x)

	if updateFunc != nil {
		if err := updateFunc(x); err != nil {
			return err
		}
	}

	f.Seek(0, 0)
	f.Truncate(0)

	data, err := json.MarshalIndent(x, "", "  ")

	if err != nil {
		return err
	}

	if _, err = f.Write(data); err != nil {
		return err
	}

	_, err = f.WriteString("\n")

	return err
}

func (x *Options) SaveConfig() error {
	cp := *x

	return x.UpdateConfig(func(opt *Options) error {
		*opt = cp
		return nil
	})
}

var options = &Options{
	Version: func() error {
		fmt.Printf("autobuild version 1.0\n")
		os.Exit(1)
		return nil
	},

	Pbuilder: "cowbuilder",

	Repository: RepositoryOptions{
		ListenPort: "8080",
	},

	UseTmpfs: false,
}

var parser = flags.NewParser(options, flags.Default)

func init() {
	options.BaseFlag = func(arg string) error {
		options.Base = arg
		options.LoadConfig()
		return nil
	}

	options.GroupFlag = func(arg string) error {
		options.Group = arg
		options.GroupId, _ = lookupGroupId(arg)

		return nil
	}

	parser.ShortDescription = "simple debian package builder"
}

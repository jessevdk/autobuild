package main

import (
	"encoding/json"
	"fmt"
	"os"
	"os/user"
	"path"
	"ponyo.epfl.ch/go/get/go/flags"
	"strconv"
	"syscall"
)

// #include <sys/file.h>
import "C"

type Distribution struct {
	Os            string   `json:"os"`
	CodeName      string   `json:"codename"`
	Architectures []string `json:"architectures"`
}

type BuildOptions struct {
	Distributions []*Distribution `json:"distributions"`
}

type Options struct {
	Base     string                 `json:"base,omitempty"`
	BaseFlag func(val string) error `short:"b" long:"base" description:"Base autobuild directory" json:"-" default:"/var/lib/autobuild"`
	Verbose  bool                   `short:"v" long:"verbose" description:"Verbose output" json:"-"`
	Version  func() error           `short:"V" long:"version" description:"Print the version" json:"-"`

	Remote string `short:"r" long:"remote" description:"Remote host for autobuild client commands" json:"remote"`

	BuildOptions BuildOptions `json:"build-options"`

	User string `json:"user,omitempty"`

	SetUser func(val string) error `short:"u" long:"user" description:"Authenticated user for autobuild communication" json:"-" default:"autobuild"`

	UserId uint32 `json:"-"`

	Pbuilder string `json:"pbuilder" no-flag:"-"`
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

	parseUser()
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

func (x *Options) UpdateConfig(updateFunc func(*Options)) error {
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

	updateFunc(x)

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

func (x *Distribution) SourceName() string {
	return fmt.Sprintf("%s/%s", x.Os, x.CodeName)
}

func (x *Distribution) BinaryName(arch string) string {
	return fmt.Sprintf("%s/%s/%s", x.Os, x.CodeName, arch)
}

var options = &Options{
	Version: func() error {
		fmt.Printf("autobuild version 1.0\n")
		os.Exit(1)
		return nil
	},

	Pbuilder: "cowbuilder",
}

var parser = flags.NewParser(options, flags.Default)

func parseUser() error {
	us, err := user.Lookup(options.User)

	if err != nil {
		return err
	}

	uid, err := strconv.ParseUint(us.Uid, 10, 32)

	if err != nil {
		return err
	}

	options.UserId = uint32(uid)
	return nil
}

func init() {
	options.BaseFlag = func(arg string) error {
		options.Base = arg
		options.LoadConfig()
		return nil
	}

	options.SetUser = func(arg string) error {
		options.User = arg
		return parseUser()
	}

	parseUser()
}

func main() {
	options.LoadConfig()

	if _, err := parser.Parse(); err != nil {
		os.Exit(1)
	}
}

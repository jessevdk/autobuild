package main

import (
	"errors"
	"fmt"
	"os"
	"os/exec"
	"path"
	"runtime"
	"strings"
	"io/ioutil"
	"bufio"
	"io"
)

type CommandInit struct {
}

func (x *CommandInit) addDistro(distro *Distribution, arch string) error {
	// Setup reprepro structure
	for _, dir := range []string{"conf", "incoming"} {
		os.MkdirAll(path.Join(options.Base, "repository", distro.Os, dir), 0755)
	}

	// Update reprepro config
	confdir := path.Join(options.Base, "repository", distro.Os, "conf")
	distroconf := path.Join(confdir, "distributions")

	f, err := os.OpenFile(distroconf, os.O_CREATE | os.O_APPEND | os.O_WRONLY, 0644)

	if err != nil {
		return err
	}

	fmt.Fprintln(f, "")
	fmt.Fprintf(f,  "Origin: %s\n", options.Repository.Origin)
	fmt.Fprintf(f,  "Label: %s\n", options.Repository.Label)
	fmt.Fprintf(f,  "Codename: %s\n", distro.CodeName)
	fmt.Fprintf(f,  "Architectures: %s source\n", arch)
	fmt.Fprintln(f, "Components: main")
	fmt.Fprintf(f,  "Description: %s Repository\n", options.Repository.Description)
	fmt.Fprintf(f,  "DebOverride: override.%s\n", distro.CodeName)
	fmt.Fprintf(f,  "DscOverride: override.%s\n", distro.CodeName)

	if len(options.Repository.SignKey) > 0 {
		fmt.Fprintf(f, "SignWith: %s\n", options.Repository.SignKey)
	}

	f.Close()

	f, err = os.Create(path.Join(confdir, fmt.Sprintf("override.%s", distro.CodeName)))

	if err != nil {
		return err
	}

	f.Write([]byte {'\n'});
	f.Close()

	f, err = os.OpenFile(path.Join(confdir, "options"), os.O_CREATE | os.O_EXCL | os.O_WRONLY, 0644)

	if err == nil {
		fmt.Fprintf(f, "basedir %s\n", path.Join(options.Base, "repository", distro.Os))
		f.Close()
	}

	return nil
}

func (x *CommandInit) addArch(distro *Distribution, arch string) error {
	// Update reprepro config with the architecture
	confdir := path.Join(options.Base, "repository", distro.Os, "conf")
	distroconf := path.Join(confdir, "distributions")

	frd, err := os.Open(distroconf)

	if err != nil {
		return err
	}

	fwr, err := ioutil.TempFile("", "autobuild-distributions")

	if err != nil {
		frd.Close()
		return err
	}

	rd := bufio.NewReader(frd)

	founddistro := false
	didsubst := false
	finds := fmt.Sprintf("Codename: %s", distro.CodeName)

	for {
		line, err := rd.ReadString('\n')
		var nonl string

		if err != nil && err != io.EOF {
			return err
		} else if err == nil {
			nonl = line[0:len(line)-1]
		} else {
			nonl = line
		}

		if founddistro && !didsubst && strings.HasPrefix(nonl, "Architectures: ") {
			fmt.Fprintf(fwr, "%s %s\n", nonl, arch)
			didsubst = true
		} else {
			fmt.Fprintf(fwr, "%s", line)
		}

		if !founddistro && !didsubst && line == finds {
			founddistro = true
		}

		if err == io.EOF {
			break
		}
	}

	fwr.Close()
	frd.Close()

	os.Rename(fwr.Name(), frd.Name())
	os.Chmod(frd.Name(), 0644)

	return nil
}

func (x *CommandInit) AddDistribution(distro *Distribution, arch string) error {
	var err error

	// Append distribution to know configured distributions
	options.UpdateConfig(func(opts *Options) {
		toadd := true

		// Add initialized distro to list of distributions
		for _, distcfg := range opts.BuildOptions.Distributions {
			if distcfg.Os == distro.Os && distcfg.CodeName == distro.CodeName {
				for _, archcfg := range distcfg.Architectures {
					if archcfg == arch {
						toadd = false
						break
					}
				}

				if toadd {
					if err = x.addArch(distcfg, arch); err != nil {
						return
					}

					// Add architecture
					distcfg.Architectures = append(distcfg.Architectures,
					                               arch)

					toadd = false
				}

				break
			}
		}

		if toadd {
			d := &Distribution{
				Os:            distro.Os,
				CodeName:      distro.CodeName,
				Architectures: []string{arch},
			}

			if err = x.addDistro(d, arch); err != nil {
				return
			}

			opts.BuildOptions.Distributions =
				append(opts.BuildOptions.Distributions, d)
		}
	})

	return err
}

func ParseDistributions(args []string) ([]*Distribution, error) {
	distros := make([]*Distribution, 0, len(args))

	for _, distro := range args {
		parts := strings.Split(distro, "/")

		if len(parts) == 1 {
			return nil, fmt.Errorf("The specified distribution `%s' is invalid (use <distro>/<codename>)",
				distro)
		}

		d := &Distribution{
			Os:       parts[0],
			CodeName: parts[1],
		}

		if len(parts) > 2 {
			d.Architectures = []string{parts[2]}
		} else if runtime.GOARCH == "386" {
			d.Architectures = []string{"i386"}
		} else if runtime.GOARCH == "amd64" {
			d.Architectures = []string{"amd64"}
		} else if runtime.GOARCH == "arm" {
			d.Architectures = []string{"arm"}
		}

		distros = append(distros, d)
	}

	return distros, nil
}

func (x *CommandInit) Execute(args []string) error {
	if len(args) == 0 {
		return errors.New("Please specify the distribution you want to build for (e.g. ubuntu/precise)")
	}

	distros, err := ParseDistributions(args)

	if err != nil {
		return err
	}

	cmdargs := []string{
		"--configfile",
		path.Join(options.Base, "etc", "pbuilderrc"),
		"--create",
	}

	for _, distro := range distros {
		cmd := exec.Command(options.Pbuilder, cmdargs...)

		distvar := fmt.Sprintf("DIST=%s/%s", distro.Os, distro.CodeName)

		for _, arch := range distro.Architectures {
			archvar := fmt.Sprintf("ARCH=%s", arch)

			cmd.Env = os.Environ()
			cmd.Env = append(cmd.Env, distvar)
			cmd.Env = append(cmd.Env, archvar)
			cmd.Env = append(cmd.Env, fmt.Sprintf("AUTOBUILD_BASE=%s", options.Base))

			cmd.Stderr = os.Stderr

			if options.Verbose {
				fmt.Printf("%s %s %s\n", distvar, archvar, strings.Join(cmd.Args, " "))
				cmd.Stdout = os.Stdout
			}

			fmt.Printf("Creating environment for %s/%s (%s)\n",
				distro.Os,
				distro.CodeName,
				arch)

			basepath := path.Join(options.Base, "pbuilder", distro.Os, distro.CodeName+"-"+arch)
			os.MkdirAll(path.Join(basepath, "aptcache"), 0755)

			if err := cmd.Run(); err != nil {
				return err
			}

			if err = x.AddDistribution(distro, arch); err != nil {
				return err
			}

			fmt.Printf("Finished creating environment in `%s'\n", basepath)
		}
	}

	return nil
}

func init() {
	parser.AddCommand("Init", "init", &CommandInit {})
}

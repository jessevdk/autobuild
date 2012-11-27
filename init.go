package main

import (
	"errors"
	"fmt"
	"os"
	"os/exec"
	"path"
	"runtime"
	"strings"
)

type CommandInit struct {
}

func (x *CommandInit) AddDistribution(distro *Distribution, arch string) {
	// Append distribution to know configured distributions
	options.UpdateConfig(func(x *Options) {
		toadd := true

		// Add initialized distro to list of distributions
		for _, distcfg := range x.BuildOptions.Distributions {
			if distcfg.Os == distro.Os && distcfg.CodeName == distro.CodeName {
				for _, archcfg := range distcfg.Architectures {
					if archcfg == arch {
						toadd = false
						break
					}
				}

				if toadd {
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

			x.BuildOptions.Distributions =
				append(x.BuildOptions.Distributions, d)
		}
	})
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

			x.AddDistribution(distro, arch)

			fmt.Printf("Finished creating environment in `%s'\n", basepath)
		}
	}

	return nil
}

func init() {
	parser.AddCommand("Init", "init", &CommandInit{})
}

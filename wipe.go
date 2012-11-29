package main

import (
	"os"
	"fmt"
	"bufio"
	"path"
)

type CommandWipe struct {
}

func (x *CommandWipe) removeRepositoryConfig(opts *Options, distro *Distribution, arch string) bool {
	for ic, distrocfg := range opts.BuildOptions.Distributions {
		if distrocfg.Os != distro.Os || distrocfg.CodeName != distro.CodeName {
			continue
		}

		for i, archcfg := range distrocfg.Architectures {
			if archcfg == arch {
				distrocfg.Architectures =
					append(distrocfg.Architectures[:i],
					       distrocfg.Architectures[i+1:]...)

				if len(distrocfg.Architectures) == 0 {
					opts.BuildOptions.Distributions =
						append(opts.BuildOptions.Distributions[:ic],
						       opts.BuildOptions.Distributions[ic+1:]...)

				}

				return true
			}
		}
	}

	return false
}

func (x *CommandWipe) wipeRepositoryFs(distro *Distribution, arch string) {
	os.RemoveAll(path.Join(options.Base, "pbuilder", distro.Os, fmt.Sprintf("%s-%s", distro.CodeName, arch)))
}

func (x *CommandWipe) wipeRepository(distro *Distribution) error {
	return options.UpdateConfig(func (opts *Options) {
		for _, arch := range distro.Architectures {
			x.removeRepositoryConfig(opts, distro, arch)
			x.wipeRepositoryFs(distro, arch)
		}
	})
}

func (x *CommandWipe) Execute(args []string) error {
	if len(args) > 0 {
		distros, err := ParseDistributions(args)

		if err != nil {
			return err
		}

		for _, distro := range distros {
			if err := x.wipeRepository(distro); err != nil {
				return err
			}
		}
	} else {
		fmt.Printf("Are you sure that you want to remove ALL of autobuild `%s'? [yN] ", options.Base)
		rd := bufio.NewReader(os.Stdin)

		s, err := rd.ReadString('\n')

		if err != nil {
			return err
		}

		s = s[0:len(s) - 1]

		if s == "Y" || s == "y" {
			if options.Base != "" && options.Base != "/" {
				return os.RemoveAll(options.Base)
			}
		}
	}

	return nil
}

func init() {
	parser.AddCommand("Wipe", "wipe", &CommandWipe{})
}

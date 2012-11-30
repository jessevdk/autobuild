package main

import (
	"errors"
	"fmt"
	"os"
	"path"
	"strings"
)

type CommandUpdate struct {
}

func (x *CommandUpdate) Execute(args []string) error {
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
		"--update",
	}

	for _, distro := range distros {
		cmd := MakeCommand(options.Pbuilder, cmdargs...)

		distvar := fmt.Sprintf("DIST=%s/%s", distro.Os, distro.CodeName)

		for _, arch := range distro.Architectures {
			if !options.BuildOptions.HasDistribution(distro, arch) {
				return fmt.Errorf("The distribution `%s/%s/%s` does not yet exist.",
					distro.Os,
					distro.CodeName,
					arch)
			}

			archvar := fmt.Sprintf("ARCH=%s", arch)

			cmd.Env = os.Environ()
			cmd.Env = append(cmd.Env, distvar)
			cmd.Env = append(cmd.Env, archvar)
			cmd.Env = append(cmd.Env, fmt.Sprintf("AUTOBUILD_BASE=%s", options.Base))

			if options.Verbose {
				fmt.Printf("%s %s %s\n", distvar, archvar, strings.Join(cmd.Args, " "))
			}

			fmt.Printf("Updating environment for %s/%s (%s)\n",
				distro.Os,
				distro.CodeName,
				arch)

			basepath := path.Join(options.Base, "pbuilder", distro.Os, distro.CodeName+"-"+arch)

			os.MkdirAll(path.Join(basepath, "aptcache"), 0755)

			if err := cmd.Run(); err != nil {
				return err
			}

			fmt.Printf("Finished updating environment in `%s'\n", basepath)
		}
	}

	return nil
}

func init() {
	parser.AddCommand("Update", "update", &CommandUpdate{})
}

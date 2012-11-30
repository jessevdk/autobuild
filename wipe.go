package main

import (
	"os"
	"fmt"
	"bufio"
	"path"
	"io/ioutil"
	"io"
	"strings"
)

type CommandWipe struct {
}

func (x *CommandWipe) removeRepositoryConfig(opts *Options, distro *Distribution, arch string) (bool, bool) {
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

				return true, len(distrocfg.Architectures) == 0
			}
		}
	}

	return false, false
}

func (x *CommandWipe) rewriteConf(confpath string, fn func (line string) (string, bool)) error {
	fr, err := os.Open(confpath)

	if err != nil {
		return err
	}

	tmp, err := ioutil.TempFile("", "autobuild-reprepro-conf")

	if err != nil {
		fr.Close()
		return err
	}

	rd := bufio.NewReader(fr)

	for {
		line, err := rd.ReadString('\n')

		if err != nil && err != io.EOF {
			tmp.Close()
			fr.Close()

			return err
		}

		if err == nil {
			line = line[0:len(line)-1]
		}

		nl, keep := fn(line)

		if keep {
			fmt.Fprintln(tmp, nl)
		}

		if err == io.EOF {
			break
		}
	}

	tmp.Close()
	fr.Close()

	if err := os.Rename(tmp.Name(), confpath); err != nil {
		return err
	}

	os.Chmod(confpath, 0644)
	return nil
}

func (x *CommandWipe) wipeCodename(distro *Distribution) {
	shoulddelete := false

	confdir := path.Join(options.Base, "repository", distro.Os, "conf")
	confpath := path.Join(confdir, "distributions")

	m := fmt.Sprintf("# %s", distro.SourceName())

	x.rewriteConf(confpath, func (line string) (string, bool) {
		if len(line) == 0 {
			return "", !shoulddelete
		}

		if shoulddelete {
			if line[0] == '#' {
				shoulddelete = false
			}
		} else if line == m {
			shoulddelete = true
		}

		return line, !shoulddelete
	})

	// Remove override file
	os.Remove(path.Join(options.Base, "repository", distro.Os, "conf", "override." + distro.CodeName))

	// Remove incoming section
	confpath = path.Join(confdir, "incoming")
	inrepo := false
	shoulddelete = false

	x.rewriteConf(confpath, func (line string) (string, bool) {
		if len(line) == 0 {
			return "", !inrepo
		}

		if inrepo {
			if line[0] == '#' {
				inrepo = false
			}
		} else if line == m {
			inrepo = true
		}

		return line, !inrepo
	})
}

func (x *CommandWipe) wipeArchitecture(distro *Distribution, arch string) {
	inrepo := false

	confdir := path.Join(options.Base, "repository", distro.Os, "conf")

	m := fmt.Sprintf("# %s", distro.SourceName())
	archs := "Architectures:"

	confpath := path.Join(confdir, "distributions")
	
	x.rewriteConf(confpath, func (line string) (string, bool) {
		if len(line) == 0 {
			return "", true
		}

		if inrepo {
			if line[0] == '#' {
				inrepo = false
			} else if strings.HasPrefix(line, archs) {
				// Remove our architecture from the list
				rest := line[len(archs):]
				line = archs

				parts := strings.Split(rest, " ")

				for _, p := range parts {
					if len(p) > 0 && p != arch {
						line += " " + p
					}
				}
			}
		} else if line == m {
			inrepo = true
		}

		return line, true
	})
}

func (x *CommandWipe) wipeRepositoryFs(distro *Distribution, arch string, opts *Options, all bool) {
	os.RemoveAll(path.Join(options.Base, "pbuilder", distro.Os, fmt.Sprintf("%s-%s", distro.CodeName, arch)))

	runReproMutex.Lock()
	defer runReproMutex.Unlock()

	repodir := path.Join(options.Base, "repository")

	if len(opts.BuildOptions.Distributions) == 0 {
		// Wipe the whole thing
		f, _ := os.Open(repodir)

		if f == nil {
			return
		}

		infos, _ := f.Readdir(-1)

		for _, info := range infos {
			if info.IsDir() {
				os.RemoveAll(path.Join(repodir, info.Name()))
			}
		}
	} else if all {
		// Check if we can remove for the whole OS
		removeos := true

		for _, distrocfg := range opts.BuildOptions.Distributions {
			if distrocfg.Os == distro.Os && distrocfg.CodeName != distro.CodeName {
				removeos = false
				break
			}
		}

		if removeos {
			// Wipe just the OS directory
			os.RemoveAll(path.Join(repodir, distro.Os))
		} else {
			// Need to update the conf to just remove this codename
			x.wipeCodename(distro)
			clearVanishedRepReproLocked(distro)
		}
	} else {
		// Remove just this architecture from codename
		x.wipeArchitecture(distro, arch)
		clearVanishedRepReproLocked(distro)
	}
}

func (x *CommandWipe) wipeRepositories(distros []*Distribution) error {
	return options.UpdateConfig(func (opts *Options) error {
		for _, distro := range distros {
			for _, arch := range distro.Architectures {
				wasconf, all := x.removeRepositoryConfig(opts, distro, arch)

				x.wipeRepositoryFs(distro, arch, opts, !wasconf || all)
			}
		}

		return nil
	})
}

func (x *CommandWipe) Execute(args []string) error {
	if len(args) > 0 {
		distros, err := ParseDistributions(args)

		if err != nil {
			return err
		}

		if err := x.wipeRepositories(distros); err != nil {
			return err
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

package main

import (
	"path"
	"sync"
)

var runReproMutex sync.Mutex

func repReproArgs(distro *Distribution) []string {
	repodir := path.Join(options.Base, "repository")

	ret := []string {
		"-b",
		path.Join(repodir, distro.Os),
		"--gnupghome",
		path.Join(options.Base, ".gnupg"),
	}

	if options.Verbose {
		ret = append(ret, "-V")
	} else {
		ret = append(ret, "--silent")
	}

	return ret
}

func runRepRepro(distro *Distribution) error {
	runReproMutex.Lock()
	defer runReproMutex.Unlock()

	args := repReproArgs(distro)
	args = append(args, "processincoming", distro.CodeName)

	cmd := MakeCommand("reprepro", args...)

	return cmd.Run()
}

func initRepRepro(distro *Distribution) error {
	runReproMutex.Lock()
	defer runReproMutex.Unlock()

	args := repReproArgs(distro)
	args = append(args, "export", distro.CodeName)

	return RunCommand("reprepro", args...)
}

func clearVanishedRepReproLocked(distro *Distribution) error {
	args := repReproArgs(distro)
	args = append(args, "clearvanished")

	return RunCommand("reprepro", args...)
}

package main

import (
	"path"
	"sync"
)

var runReproMutex sync.Mutex

func runRepRepro(distro Distribution) error {
	repodir := path.Join(options.Base, "repository")

	runReproMutex.Lock()
	defer runReproMutex.Unlock()

	cmd := prepareCommand("reprepro",
	                      "-b",
	                      path.Join(repodir, distro.Os),
	                      "--gnupghome",
	                      path.Join(options.Base, ".gnupg"),
	                      "processincoming",
	                      distro.CodeName)

	return cmd.Run()
}

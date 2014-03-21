# Autobuild
autobuild provides an easy way to manage a small debian (apt) build system and
repository, without the hassle of configuring all the various components.
Managing a debian repository becomes as easy as a one time, guided configuration
and simple commands to build and release packages. autobuild uses
cowbuilder/pbuilder to build packages, and uses reprepro to manage the
repository.

autobuild is designed to be self-contained and standalone. The `autobuild`
binary has no external dependencies and can be easily deployed. Furthermore,
all files related to autobuild will be maintained under one directory tree
(`/var/lib/autobuild` by default) instead of spread out over the system. It
is therefore easy to move autobuild installations to different machines, back
them up or remove them completely from a system. Note however that autobuild
does use things like cowbuilder/pbuilder and reprepro which should be installed
on the system. The one-time configuration will automatically install these
dependencies if not yet installed.

# Building
You will need to have go >= 1.1 installed before building autobuild. If you
have go installed, then simply run the usual configure/make:

    ./configure
    make

This will result in a single, standalone binary, called `autobuild`. This binary
does not have any runtime dependencies, so it can be simply copied to your target
system if needed. You can also use `make install` which will install the binary
to `/usr/local/bin` (unless otherwise configured) and a man page.

The `autobuild` binary serves as both the daemon and client, through various
commands (see the man page or --help).

# One-time configuration
Before you can start using autobuild, you will have to do a one-time configuration
to setup autobuild. Running `sudo autobuild install` will first install the
necessary dependencies such as cowbuilder and reprepro. It will then continue
with asking for the repository name, configuring the gpg signing key etc.
After the install procedure is finished, the basic autobuild setup can be found
at `/var/lib/autobuild` and you are ready to continue using autobuild. You should
normally not have to repeat this step.

# Usage

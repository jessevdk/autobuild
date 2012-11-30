package main

import (
	"fmt"
	"os"
	"path"
	"regexp"
	"time"
	"io/ioutil"
	"strings"
	"bufio"
	"io"
)

type CommandGitPreparePackage struct {
}

func (x *CommandGitPreparePackage) updateChangelog(name string, version string) error {
	f, err := os.Open("debian/changelog")

	if err != nil {
		return err
	}

	changelog, err := ioutil.ReadAll(f)
	f.Close()

	if err != nil {
		return err
	}

	date := time.Now().Format(time.RFC1123Z)
	user, err := MakeCommand("git", "config", "--get", "user.name").Output()

	if err != nil {
		return err
	}

	email, err := MakeCommand("git", "config", "--get", "user.email").Output()

	if err != nil {
		return err
	}

	ch := fmt.Sprintf("%s (%s-1) UNRELEASED; urgency=low\n\n  * \n\n -- %s <%s>  %s\n", name, version, user, email, date) + string(changelog)

	f, err = os.Create("debian/changelog")

	if err != nil {
		return err
	}

	f.Write([]byte(ch))
	f.Close()

	editor := os.Getenv("EDITOR")

	if len(editor) == 0 {
		editor = "vim"
	}

	RunCommand(editor, "debian/changelog")
	RunCommand("git", "add", "debian/changelog")
	RunCommand("git", "commit", "-e", "-m", fmt.Sprintf("Release version %s", version))

	return nil
}

func (x *CommandGitPreparePackage) readDebianBranches() ([]string, error) {
	f, err := os.Open(".git/refs/heads")

	if err != nil {
		return nil, err
	}

	names, err := f.Readdirnames(0)

	if err != nil {
		return nil, err
	}

	ret := make([]string, 0)

	for _, n := range names {
		if strings.HasPrefix(n, "debian-") {
			ret = append(ret, n[7:])
		}
	}

	return ret, nil
}

func (x *CommandGitPreparePackage) Execute(args []string) error {
	if len(args) != 1 {
		return fmt.Errorf("Please provide one tarball of a package (e.g. made with 'make distcheck')")
	}

	matched := packageInfoRegex.FindStringSubmatch(args[0])

	if matched == nil {
		return fmt.Errorf("The package `%s' does not appear to be a package...", args[0])
	}

	name := matched[1]
	version := matched[2]
	compression := matched[4]

	f, err := os.Open("debian/changelog")

	if err != nil {
		return err
	}

	reader := bufio.NewReader(f)
	line, err := reader.ReadString('\n')

	if err != nil {
		return err
	}

	f.Close()

	chm, _ := regexp.Compile(`^([^\s]+)\s+\(([0-9]+(\.[0-9]+)+)-[0-9]\)`)
	matched = chm.FindStringSubmatch(line)

	if matched == nil {
		return fmt.Errorf("Failed to extract version information from debian changelog")
	}

	if matched[1] != name || matched[2] != version {
		if err := x.updateChangelog(name, version); err != nil {
			return err
		}
	}

	tmp, err := ioutil.TempDir("", "git-prepare-package")

	if err != nil {
		return err
	}

	defer os.RemoveAll(tmp)

	var tp string

	switch compression {
	case "gz":
		tp = "z"
	case "bz2":
		tp = "j"
	case "xz":
		tp = "J"
	}

	// Extract
	err = RunCommand("tar", "-C", tmp, fmt.Sprintf("-x%sf", tp), args[0])

	if err != nil {
		return err
	}

	nm := fmt.Sprintf("%s_%s", name, version)
	orig := fmt.Sprintf("%s.orig.tar.gz", nm)
	diff := fmt.Sprintf("%s.diff", nm)
	diffgz := fmt.Sprintf("%s.gz", diff)
	dname := fmt.Sprintf("%s-%s", name, version)

	err = RunCommand("tar", "-C", tmp, "-czf", path.Join(tmp, orig), dname)

	if err != nil {
		return err
	}

	os.RemoveAll(dname)
	os.Mkdir(path.Join(tmp, "patches"), 0755)

	branches, err := x.readDebianBranches()

	if err != nil {
		return err
	}

	files := make([]string, 0)

	for _, branch := range branches {
		if options.Verbose {
			fmt.Printf("Generating patches for `%s'\n", branch)
		}

		cmd := MakeCommand("git", "diff", fmt.Sprintf("debian..debian-%s", branch))
		xz := MakeCommand("xz", "-z", "-c")

		xz.Stdout, _ = os.Create(fmt.Sprintf("%s/patches/%s.diff.xz", tmp, branch))
		xz.Stdin, _ = cmd.StdoutPipe()

		go cmd.Run()
		go xz.Run()

		cmd.Wait()
		xz.Wait()

		files = append(files, fmt.Sprintf("patches/%s.diff.xz", branch))
	}

	rd, err := os.Open("debian/autobuild/options")

	if err == nil {
		wr, err := os.Create(path.Join(tmp, "options"))

		if err != nil {
			rd.Close()
			return err
		}

		io.Copy(wr, rd)

		wr.Close()
		rd.Close()

		files = append(files, "options")
	}

	diffbase, err := MakeCommand("git", "merge-base", "master", "debian").Output()

	if err != nil {
		return err
	}

	b := strings.TrimRight(string(diffbase), "\n")

	diffcmd := MakeCommand("git", "diff", fmt.Sprintf("%s..debian", b))
	gzipcmd := MakeCommand("gzip", "-c")

	gzipcmd.Stdin, _ = diffcmd.StdoutPipe()
	gzipcmd.Stdout, _ = os.Create(path.Join(tmp, diffgz))

	go diffcmd.Run()
	go gzipcmd.Run()

	diffcmd.Wait()
	gzipcmd.Wait()

	targs := []string {
		"-C",
		tmp,
		"-cJf",
		fmt.Sprintf("%s.tar.xz", nm),
		orig,
		diffgz,
	}

	targs = append(targs, files...)

	if err := RunCommand("tar", targs...); err != nil {
		return err
	}

	fmt.Printf("Generated %s.tar.xz\n", nm)

	return nil
}

func init() {
	parser.AddCommand("Git prepare package", "git-prepare-package", &CommandGitPreparePackage{})
}

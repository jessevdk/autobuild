package main

import (
	"bufio"
	"fmt"
	"os"
	"path"
	"strconv"
	"strings"
)

type CommandRelease struct {
}

func (x *CommandRelease) Execute(args []string) error {
	a := &Incoming{}

	ret := &IncomingReply{}

	if err := RemoteCall("DaemonCommands.Incoming", a, ret); err != nil {
		return err
	}

	if len(ret.Packages) == 0 {
		fmt.Println("There are no packages staged to be released...")
		return nil
	}

	fmt.Println("Packages ready to be released:")
	fmt.Println()

	longest := len(fmt.Sprintf("%d", len(ret.Packages)))

	for i, r := range ret.Packages {
		n := fmt.Sprintf("%d", i+1)
		pad := strings.Repeat(" ", longest-len(n))

		fmt.Printf("  %s%s) %s/%s %s %s\n",
			pad,
			n,
			r.Distribution.Os,
			r.Distribution.CodeName,
			r.Distribution.Architectures[0],
			path.Base(r.Name))

		for _, f := range r.Files {
			fmt.Printf("  %s%s\n", strings.Repeat(" ", longest+4), path.Base(f))
		}

		fmt.Println()
	}

	fmt.Printf("Which packages do you want to release? ")

	rd := bufio.NewReader(os.Stdin)
	line, err := rd.ReadString('\n')

	if err != nil {
		return err
	}

	line = line[0 : len(line)-1]

	packages := make([]IncomingPackage, 0)

	parts := strings.Split(line, ",")

	for _, part := range parts {
		part = strings.TrimSpace(part)

		if part == "*" {
			packages = ret.Packages
			break
		}

		rng := strings.SplitN(part, ":", 2)

		if len(rng) == 1 {
			idx, err := strconv.ParseInt(part, 10, 32)

			if err != nil {
				return err
			}

			packages = append(packages, ret.Packages[int(idx)-1])
		} else {
			start, err := strconv.ParseInt(rng[0], 10, 32)

			if err != nil {
				return err
			}

			end, err := strconv.ParseInt(rng[1], 10, 32)

			if err != nil {
				return err
			}

			packages = append(packages, ret.Packages[start:end+1]...)
		}
	}

	rel := &Release{
		Packages: packages,
	}

	r := &ReleaseReply{}
	return RemoteCall("DaemonCommands.Release", rel, r)
}

func init() {
	parser.AddCommand("release",
		"Release packages that have been built",
		"The release command releases packages that have finished building. You will be presented with a list of finished packages and you can choose which packages to release. Note that you can specify packages by a comma separated list of their number (e.g. 1,2), ranges (e.g. 1:3) or use `*' to release all packages.",
		&CommandRelease{})
}

package main

import (
	"fmt"
	"os"
	"path"
	"bufio"
	"strings"
	"strconv"
	"io/ioutil"
)

type CommandInstall struct {
}

func (x *CommandInstall) makeGroup() (int, error) {
	if len(options.Group) == 0 {
		return 0, nil
	}

	g, err := lookupGroupId(options.Group)

	if err != nil {
		fmt.Printf("Creating group `%s'\n", options.Group)
		opts := []string {}

		if !options.Verbose {
			opts = append(opts, "--quiet")
		}

		opts = append(opts, options.Group)
		cmd := MakeCommand("addgroup", opts...)

		if err := cmd.Run(); err != nil {
			return 0, err
		}

		g, err = lookupGroupId(options.Group)

		if err != nil {
			return 0, err
		}
	}

	return int(g), nil
}

func (x *CommandInstall) readOption(q string, retval *string) error {
	rd := bufio.NewReader(os.Stdin)

	fmt.Printf("%s: ", q)

	if *retval != "" {
		fmt.Printf("[%s] ", *retval)
	}

	line, err := rd.ReadString('\n')

	if err != nil {
		return err
	}

	line = line[0:len(line)-1]

	if len(line) != 0 {
		*retval = line
	}

	return nil
}

type SignKey struct {
	Id string
	Date string
	Name string
}

func (x *CommandInstall) parseSignKeys(s string) []SignKey {
	parts := strings.Split(string(s), "\n")
	ret := make([]SignKey, 0)

	for _, part := range parts {
		fields := strings.Split(part, ":")

		if fields[0] == "pub" {
			ret = append(ret, SignKey {
				Id: fields[4],
				Date: fields[5],
				Name: fields[9],
			})
		}
	}

	return ret
}

func (x *CommandInstall) listSignKeys() ([]SignKey, error) {
	gpgdir := path.Join(options.Base, ".gnupg")

	args := []string {
		"--homedir", gpgdir,
		"--list-public-keys",
		"--with-colons",
	}

	if !options.Verbose {
		args = append(args, "-q")
	}

	cmd := MakeCommand("gpg", args...)

	ret, err := cmd.Output()

	if err != nil {
		return nil, err
	}

	return x.parseSignKeys(string(ret)), nil
}

func (x *CommandInstall) exportSignKey() error {
	if len(options.Repository.SignKey) == 0 {
		return nil
	}

	gpgdir := path.Join(options.Base, ".gnupg")
	keyfile := path.Join(options.Base, "repository", "sign.key")

	args := []string {
		"--homedir", gpgdir,
		"--armor",
		"--output", keyfile,
		"--export",
		options.Repository.SignKey,
	}

	if !options.Verbose {
		args = append(args, "-q")
	}

	os.Remove(keyfile)

	cmd := MakeCommand("gpg", args...)

	return cmd.Run()

}

func (x *CommandInstall) configureSignKey() error {
	if len(options.Repository.SignKey) > 0 {
		return nil
	}

	signkeys, err := x.listSignKeys()

	if err != nil {
		return err
	}

	fmt.Println()
	fmt.Println("  2.4 APT repository sign key")

	if len(signkeys) > 0 {
		fmt.Println()
		fmt.Println("       You repository will need a sign key, choose one of your existing keys or a new key:")
		fmt.Println()

		for i, key := range signkeys {
			fmt.Printf("         %d) %s  %s, %s\n", i + 1, key.Id, key.Date, key.Name)
		}

		fmt.Printf("         %d) Generate a new key\n", len(signkeys) + 1)
		fmt.Println()

		for {
			fmt.Printf("         Key to use: [generate new key] ");
			rd := bufio.NewReader(os.Stdin)

			s, err := rd.ReadString('\n')

			if err != nil {
				return err
			}

			s = s[0:len(s) - 1]

			if len(s) == 0 {
				fmt.Println()
				break
			}

			choice, err := strconv.ParseInt(s, 10, 32)

			if err != nil || choice < 1 || int(choice) > len(signkeys) + 1 {
				continue
			}

			choice -= 1

			if int(choice) == len(signkeys) {
				fmt.Println()
			} else {
				options.Repository.SignKey = signkeys[choice].Id
			}

			break
		}
	}

	if len(options.Repository.SignKey) == 0 {
		fmt.Println("       I'm going to generate a new signing key for the repository.")

		var name string
		var email string

		name = options.Repository.Label

		fmt.Println()

		if err := x.readOption("         Name", &name); err != nil {
			return err
		}

		for len(email) == 0 {
			if err := x.readOption("         E-mail", &email); err != nil {
				return err
			}
		}

		gpgdir := path.Join(options.Base, ".gnupg")

		args := []string {
			"--homedir", gpgdir,
			"--gen-key",
			"--batch",
		}

		if !options.Verbose {
			args = append(args, "-q")
		}

		cmd := MakeCommand("gpg", args...)

		pipe, err := cmd.StdinPipe()

		if err != nil {
			return err
		}

		if err = cmd.Start(); err != nil {
			return err
		}

		tmp, err := ioutil.TempDir("", "autobuild-gpg")

		if err != nil {
			return err
		}

		defer os.RemoveAll(tmp)

		pubring := path.Join(tmp, "pubring.pub")
		secring := path.Join(tmp, "secring.sec")

		fmt.Fprintln(pipe, "Key-Type: RSA")
		fmt.Fprintln(pipe, "Key-Length: 2048")
		fmt.Fprintf(pipe, "Name-Real: %s\n", name)
		fmt.Fprintf(pipe, "Name-Email: %s\n", email)
		fmt.Fprintln(pipe, "Expire-Date: 0")
		fmt.Fprintf(pipe, "%%pubring %s\n", pubring)
		fmt.Fprintf(pipe, "%%secring %s\n", secring)

		pipe.Close()

		if err = cmd.Wait(); err != nil {
			fmt.Printf("Failed: %s\n", err)
			return err
		}

		args = []string {
			"--homedir", gpgdir,
			"--no-default-keyring",
			"--secret-keyring", secring,
			"--keyring", pubring,
			"--list-public-keys",
			"--with-colon",
		}

		if !options.Verbose {
			args = append(args, "-q")
		}

		cmd = MakeCommand("gpg", args...)

		s, err := cmd.Output()

		if err != nil {
			return err
		}

		newkeys := x.parseSignKeys(string(s))

		if len(newkeys) == 0 {
			return fmt.Errorf("Failed to generate key")
		}

		if options.Verbose {
			fmt.Printf("       Importing key: %s  %s, %s\n",
			           newkeys[0].Id,
			           newkeys[0].Date,
			           newkeys[0].Name)
		}

		args = []string {
			"--homedir", gpgdir,
		}

		if options.Verbose {
			args = append(args, "-q")
		}

		args = append(args, "--import", pubring, secring)

		cmd = MakeCommand("gpg", args...)
		err = cmd.Run()

		if err != nil {
			return err
		}

		options.Repository.SignKey = newkeys[0].Id
		fmt.Printf("       Generated sign key: %s\n", newkeys[0].Id)
	}

	return nil
}

func (x *CommandInstall) firstTimeConfiguration() error {
	os.MkdirAll(path.Join(options.Base, "repository"), 0755)

	fmt.Println()
	fmt.Println("Please answer the following questions to configure autobuild. Default values are shown in [] if applicable (press <enter> to accept the default value)")
	fmt.Println()
	fmt.Println("1. General options")

	if err := x.readOption("  1.1 Install location", &options.Base); err != nil {
		return err
	}

	if err := x.readOption("  1.2 Group", &options.Group); err != nil {
		return err
	}

	fmt.Println()
	fmt.Println("2. Repository configuration")

	if err := x.readOption("  2.1 APT repository origin", &options.Repository.Origin); err != nil {
		return err
	}

	if len(options.Repository.Label) == 0 {
		options.Repository.Label = options.Repository.Origin
	}

	if err := x.readOption("  2.2 APT repository label", &options.Repository.Label); err != nil {
		return err
	}

	if err := x.readOption("  2.3 APT repository description", &options.Repository.Description); err != nil {
		return err
	}

	if err := x.configureSignKey(); err != nil {
		return err
	}

	if err := x.exportSignKey(); err != nil {
		return err
	}

	options.SaveConfig()
	fmt.Println()

	return nil
}

func (x *CommandInstall) Execute(args []string) error {
	pkgs := []string{
		options.Pbuilder,
		"devscripts",
		"reprepro",
		"debootstrap",
		"debian-archive-keyring",
		"ubuntu-keyring",
		"bzip2",
		"gzip",
		"xz-utils",
		"patch",
	}

	aptargs := []string{
		"install",
		"-y",
	}

	if !options.Verbose {
		aptargs = append(aptargs, "-q")
	}

	fmt.Println("Installing dependencies")
	RunCommand("apt-get", append(aptargs, pkgs...)...)

	if err := x.firstTimeConfiguration(); err != nil {
		return err
	}

	_, err := x.makeGroup()

	if err != nil {
		return err
	}

	WriteResource("pbuilderrc", path.Join(options.Base, "etc", "pbuilderrc"))

	updatehook := path.Join(options.Base, "pbuilder", "hooks", "D10apt-get-update")
	WriteResource("D10apt-get-update", updatehook)

	// Make hook executable
	os.Chmod(updatehook, 0755)

	// Create dirs
	for _, dir := range []string{"repository", "pbuilder"} {
		os.MkdirAll(path.Join(options.Base, dir), 0755)
	}

	for _, dir := range []string{"ccache", "aptcache", "result", "hooks"} {
		os.MkdirAll(path.Join(options.Base, "pbuilder", dir), 0755)
	}

	fmt.Printf("Installation complete. autobuild has been setup in `%s'. You can change the autobuild configuration by editing the etc/autobuild.json file in this directory, or by using the `autobuild config' command.\n",
	            options.Base)

	fmt.Println()
	fmt.Printf("Your repository will be available on `http://localhost:%s' when running the daemon. You can change the port in the configuration if you want. You should proxy your frontend webserver (e.g. apache, nginx, cherokee, etc.) to this webserver. The repository public sign key is available at `http://localhost:%s/sign.key'.\n",
	           options.Repository.ListenPort,
	           options.Repository.ListenPort)

	fmt.Println()
	fmt.Println("The debian repositories will be available using the following apt deb line:")
	fmt.Printf("  deb http://localhost:%s/<distribution>/ <codename> main\n", options.Repository.ListenPort)
	fmt.Printf("  deb-src http://localhost:%s/<distribution>/ <codename> main\n", options.Repository.ListenPort)
	fmt.Println()
	fmt.Printf("Where <distribution> is debian or ubuntu and <codename> is precise or wheezy (for example). You should obviously substitute localhost:%s for your own publicly available domain name from where you proxy the autobuild repository webserver.\n", options.Repository.ListenPort)
	fmt.Println()
	fmt.Println("Please refer to `autobuild init' for more information on setting up autobuild for specific distributions.")
	fmt.Println()

	return nil
}

func init() {
	parser.AddCommand("Install", "install", &CommandInstall{})
}

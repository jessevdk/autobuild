package main

import (
	"bytes"
	"os"
	"fmt"
	"github.com/jessevdk/go-flags"
	"io"
	"strings"
	"io/ioutil"
	"compress/gzip"
)

func main() {
	var options = struct {
		Package string `short:"p" long:"package" description:"Which package to output the resources for"`
		Variable string `short:"v" long:"variable" description:"The variable name containing the resources"`
		Output string `short:"o" long:"output" description:"File to write the result to"`
		StripPrefix string `short:"s" long:"strip-prefix" description:"Strip specified prefix from resource names"`
		Compress bool `short:"c" long:"compress" description:"Compress files using gzip"`
	} {
		Package: "main",
		Variable: "Resources",
		Output: "-",
		StripPrefix: "",
		Compress: false,
	}

	args, err := flags.Parse(&options)

	if err != nil {
		os.Exit(1)
	}

	var writer io.WriteCloser

	if options.Output == "-" {
		writer = os.Stdout
	} else {
		writer, err = os.Create(options.Output)

		if err != nil {
			fmt.Fprintf(os.Stderr, "Failed to create output file %s: %v\n", options.Output, err)
			os.Exit(1)
		}
	}

	ret := make(map[string][]byte)

	for _, p := range args {
		f, err := os.Open(p)

		if err != nil {
			fmt.Fprintf(os.Stderr, "Error opening %s: %v\n", p, err)
			os.Exit(1)
		}

		var b []byte

		if options.Compress {
			var bwriter bytes.Buffer

			w := gzip.NewWriter(&bwriter)
			_, err = io.Copy(w, f)

			w.Close()

			if err == nil {
				b = bwriter.Bytes()
			}
		} else {
			b, err = ioutil.ReadAll(f)
		}

		f.Close()

		if err != nil {
			fmt.Fprintf(os.Stderr, "Error reading %s: %v\n", p, err)
			os.Exit(1)
		}

		if len(options.StripPrefix) != 0 && strings.HasPrefix(p, options.StripPrefix) {
			p = p[len(options.StripPrefix):]
		}

		ret[p] = b
	}

	fmt.Fprintf(writer, "package %s\n\n", options.Package)
	fmt.Fprintf(writer, "var %s = %#v\n", options.Variable, ret)

	writer.Close()
}

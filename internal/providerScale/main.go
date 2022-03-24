package main

import (
	"os"

	"github.com/crossplane/conformance/internal/providerScale/cmd"

	"github.com/spf13/pflag"
)

func main() {
	pflag.CommandLine = pflag.NewFlagSet("quantify", pflag.ExitOnError)
	root := cmd.NewCmdQuantify()
	if err := root.Execute(); err != nil {
		os.Exit(1)
	}
}

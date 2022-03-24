package main

import (
	"github.com/crossplane/conformance/internal/providerScale/cmd"
	"os"

	"github.com/spf13/pflag"
)

func main() {
	pflag.CommandLine = pflag.NewFlagSet("quantify", pflag.ExitOnError)
	root := cmd.NewCmdQuantify()
	if err := root.Execute(); err != nil {
		os.Exit(1)
	}
}

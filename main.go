package main

import (
	"fmt"
	"os"

	"github.com/mitchellh/cli"
)

func main() {
	cli := &cli.CLI{
		Args:     os.Args[1:],
		Commands: Commands,
		HelpFunc: helpFunc,
	}

	exitCode, err := cli.Run()
	if err != nil {
		fmt.Fprintf(os.Stderr, "nitro: unexpected error: %v\n", err)
		os.Exit(1)
	}

	os.Exit(exitCode)
}

func helpFunc(commands map[string]cli.CommandFactory) string {
	basicHelp := cli.BasicHelpFunc("nitro")(commands)
	return fmt.Sprintf("nitro is a command-line interface to Nitrous.IO.\n\n%s", basicHelp)
}

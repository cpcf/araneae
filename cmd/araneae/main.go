package main

import (
	"errors"
	"fmt"
	"os"

	"github.com/cpcf/araneae/internal/cli"
)

func main() {
	if err := run(os.Args[1:]); err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}
}

func run(args []string) error {
	if len(args) == 0 {
		return errors.New("usage: araneae <scan|check|serve> [flags]")
	}

	switch args[0] {
	case "scan":
		return cli.RunScan(args[1:])
	case "check":
		return cli.RunCheck(args[1:])
	case "serve":
		return cli.RunServe(args[1:])
	case "-h", "--help", "help":
		fmt.Fprintln(os.Stdout, usage())
		return nil
	default:
		return fmt.Errorf("unknown command %q\n\n%s", args[0], usage())
	}
}

func usage() string {
	return `usage:
  araneae scan <entry-url> [flags]
  araneae check <entry-url> [flags]
  araneae serve <report-path> [flags]`
}

package cli

import (
	"flag"
	"fmt"
	"strings"
)

type boolFlag interface {
	IsBoolFlag() bool
}

func interspersePositionals(fs *flag.FlagSet, args []string) ([]string, error) {
	flags := make([]string, 0, len(args))
	positionals := make([]string, 0, len(args))

	for i := 0; i < len(args); i++ {
		arg := args[i]
		if arg == "--" {
			positionals = append(positionals, args[i+1:]...)
			break
		}
		if !looksLikeFlag(arg) {
			positionals = append(positionals, arg)
			continue
		}

		name := flagName(arg)
		if name == "" {
			positionals = append(positionals, arg)
			continue
		}

		defined := fs.Lookup(name)
		if defined == nil {
			return nil, fmt.Errorf("flag provided but not defined: -%s", name)
		}

		flags = append(flags, arg)
		if strings.Contains(arg, "=") {
			continue
		}
		if bf, ok := defined.Value.(boolFlag); ok && bf.IsBoolFlag() {
			continue
		}
		if i+1 >= len(args) {
			continue
		}
		i++
		flags = append(flags, args[i])
	}

	return append(flags, positionals...), nil
}

func looksLikeFlag(arg string) bool {
	return strings.HasPrefix(arg, "-") && arg != "-"
}

func flagName(arg string) string {
	trimmed := strings.TrimLeft(arg, "-")
	name, _, _ := strings.Cut(trimmed, "=")
	return name
}

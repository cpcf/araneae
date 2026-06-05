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
		if name == "h" || name == "help" {
			flags = append(flags, arg)
			continue
		}

		defined := fs.Lookup(name)
		if defined == nil {
			if followsHeaderValue(args, i) {
				return nil, fmt.Errorf("flag provided but not defined after --header; quote the full header value")
			}
			return nil, fmt.Errorf("flag provided but not defined: -%s", name)
		}

		flags = append(flags, arg)
		if strings.Contains(arg, "=") {
			if name == "header" && looksLikeSplitHeaderValue(inlineFlagValue(arg), args, i+1, len(positionals) > 0) {
				return nil, fmt.Errorf("--header value looks split; quote the full header value")
			}
			continue
		}
		if bf, ok := defined.Value.(boolFlag); ok && bf.IsBoolFlag() {
			continue
		}
		if i+1 >= len(args) {
			continue
		}
		if name == "header" && looksLikeSplitHeaderValue(args[i+1], args, i+2, len(positionals) > 0) {
			return nil, fmt.Errorf("--header value looks split; quote the full header value")
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

func inlineFlagValue(arg string) string {
	_, value, _ := strings.Cut(arg, "=")
	return value
}

func followsHeaderValue(args []string, index int) bool {
	if index >= 2 && flagName(args[index-2]) == "header" {
		return true
	}
	if index >= 1 && looksLikeFlag(args[index-1]) && flagName(args[index-1]) == "header" && strings.Contains(args[index-1], "=") {
		return true
	}
	return false
}

func looksLikeSplitHeaderValue(value string, args []string, next int, havePositionals bool) bool {
	if next >= len(args) {
		return false
	}
	if !strings.HasSuffix(strings.TrimSpace(value), ":") {
		return false
	}
	if looksLikeFlag(args[next]) {
		return true
	}
	if havePositionals {
		return true
	}
	if looksLikeAbsoluteURL(args[next]) {
		return false
	}
	for _, arg := range args[next+1:] {
		if looksLikeFlag(arg) {
			return true
		}
	}
	return false
}

func looksLikeAbsoluteURL(arg string) bool {
	return strings.HasPrefix(arg, "http://") || strings.HasPrefix(arg, "https://")
}

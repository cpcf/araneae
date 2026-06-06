package cli

import (
	"bytes"
	"errors"
	"flag"
	"fmt"
	"io"
)

func helpRequested(err error) bool {
	return errors.Is(err, flag.ErrHelp)
}

func writeHelp(w io.Writer, usage string) error {
	_, err := io.WriteString(w, usage)
	return err
}

func flagUsage(command, positional string, fs *flag.FlagSet) string {
	var out bytes.Buffer
	fmt.Fprintf(&out, "usage: araneae %s %s [flags]\n\nFlags:\n", command, positional)
	fs.SetOutput(&out)
	fs.PrintDefaults()
	return out.String()
}

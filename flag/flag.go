// Simple helpers for extracting flags
package flag

import "flag"

type Flag struct {
	Name, Usage string
}

func Read(args []string, options func(fs *flag.FlagSet)) {
	fs := flag.NewFlagSet("", flag.ExitOnError)
	options(fs)
	fs.Parse(args)
}

func ReadString(args []string, name string, usage string) string {
	fs := flag.NewFlagSet("", flag.ContinueOnError)
	fs.Usage = func() {} // silence errors
	got := fs.String(name, "", usage)
	fs.Parse(args)
	return *got
}

func ReadBool(args []string, name string, usage string) bool {
	fs := flag.NewFlagSet("", flag.ContinueOnError)
	fs.Usage = func() {} // silence errors
	got := fs.Bool(name, false, usage)
	fs.Parse(args)
	return *got
}

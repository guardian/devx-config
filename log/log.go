// A simple logging interface that wraps the standard library 'log' package to
// add a debug level. Note, this is not, therefore, a high-performance library.
// If you need that, use something like
// https://pkg.go.dev/github.com/golang/glog.
package log

import (
	"fmt"
	stdLog "log"
)

type Logger struct {
	debug bool
}

func New(debug bool) Logger {
	return Logger{debug}
}

// For stuff users care about - wraps fmt. Always adds a trailing newline.
func (l Logger) Infof(format string, args ...any) {
	fmt.Printf(format+"\n", args...)
}

// For stuff developers care about - wraps log and only logs if debug is true.
func (l Logger) Debugf(format string, args ...any) {
	if l.debug {
		stdLog.Printf(format, args...)
	}
}

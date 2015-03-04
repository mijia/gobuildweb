package loggers

import (
	"io"
	"log"
	"os"

	"github.com/agtorre/gocolorize"
)

type ColoredLogger struct {
	c gocolorize.Colorize
	w io.Writer
}

func (cl *ColoredLogger) Write(p []byte) (n int, err error) {
	return cl.w.Write([]byte(cl.c.Paint(string(p))))
}

var (
	INFO    *log.Logger
	SUCC    *log.Logger
	WARN    *log.Logger
	ERROR   *log.Logger
	IsDebug bool
)

func Debug(format string, args ...interface{}) {
	if IsDebug {
		INFO.Printf(format, args...)
	}
}

func Error(format string, args ...interface{}) {
	ERROR.Printf(format, args...)
}

func Warn(format string, args ...interface{}) {
	WARN.Printf(format, args...)
}

func Succ(format string, args ...interface{}) {
	SUCC.Printf(format, args...)
}

func Info(format string, args ...interface{}) {
	INFO.Printf(format, args...)
}

func init() {
	INFO = log.New(os.Stdout, "(gbw) [INFO] ", 0)
	SUCC = log.New(&ColoredLogger{gocolorize.NewColor("green"), os.Stdout}, "(gbw) [SUCC] ", 0)
	WARN = log.New(&ColoredLogger{gocolorize.NewColor("yellow"), os.Stdout}, "(gbw) [WARN] ", 0)
	ERROR = log.New(&ColoredLogger{gocolorize.NewColor("red"), os.Stdout}, "(gbw) [ERROR] ", 0)
}

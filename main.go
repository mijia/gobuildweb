package main

import (
	"flag"
	"fmt"
	"io"
	"log"
	"os"

	"github.com/BurntSushi/toml"
	"github.com/agtorre/gocolorize"
)

type ProjectConfig struct {
	Package PackageConfig
}

type PackageConfig struct {
	Name         string
	Version      string
	Authors      []string
	Dependencies []string `toml:"deps"`
	OmitTests    []string `toml:"omit_tests"`
}

func main() {
	cmds := map[string]Command{
		"run":    dummyCommand,
		"test":   dummyCommand,
		"dist":   dummyCommand,
		"update": dummyCommand,
	}
	flag.Parse()
	args := flag.Args()
	if len(args) == 0 {
		usage()
	}
	if cmd, ok := cmds[args[0]]; !ok {
		usage()
	} else {
		if fi, err := os.Stat("project.toml"); os.IsNotExist(err) {
			ERROR.Fatalf("Please provide a project.toml for web project.")
		} else if err != nil {
			ERROR.Fatalf("Accessing project.toml file error, %v.", err)
		} else if fi.IsDir() {
			ERROR.Fatalf("project.toml cannot be a directory.")
		}

		if _, err := toml.DecodeFile("project.toml", &rootConfig); err != nil {
			ERROR.Fatalf("Cannot decode the project.toml into TOML format, %v", err)
		}
		INFO.Printf("Loaded project data from project.toml...")

		if err := cmd(args[1:]); err != nil {
			ERROR.Fatalf("Executing command [%v] error, %v", args[0], err)
		}
	}
}

func usage() {
	fmt.Println("Go build web provides tool to simple way to build your web application.")
	fmt.Println("Usage:")
	fmt.Println("  update\t\tUpdate all your dependencies...")
	fmt.Println("  test\t\tTest your modules")
	fmt.Println("  build\t\tBuild your web application")
	fmt.Println("  run\t\tWill watch your file changes and run the application")
	os.Exit(1)
}

type ColoredLogger struct {
	c gocolorize.Colorize
	w io.Writer
}

func (cl *ColoredLogger) Write(p []byte) (n int, err error) {
	return cl.w.Write([]byte(cl.c.Paint(string(p))))
}

var rootConfig ProjectConfig
var (
	TRACE *log.Logger
	INFO  *log.Logger
	WARN  *log.Logger
	ERROR *log.Logger
)

func init() {
	TRACE = log.New(&ColoredLogger{gocolorize.NewColor("magenta"), os.Stdout}, "[TRACE] ", log.LstdFlags|log.Lshortfile)
	INFO = log.New(&ColoredLogger{gocolorize.NewColor("green"), os.Stdout}, "[INFO] ", log.LstdFlags|log.Lshortfile)
	WARN = log.New(&ColoredLogger{gocolorize.NewColor("yellow"), os.Stdout}, "[WARN] ", log.LstdFlags|log.Lshortfile)
	ERROR = log.New(&ColoredLogger{gocolorize.NewColor("red"), os.Stdout}, "[ERROR] ", log.LstdFlags|log.Lshortfile)

	log.SetFlags(log.LstdFlags | log.Lshortfile)
}

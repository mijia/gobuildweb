package main

import (
	"flag"
	"fmt"
	"io"
	"log"
	"os"
	"runtime"
	"sync"

	"github.com/BurntSushi/toml"
	"github.com/agtorre/gocolorize"
)

type ProjectConfig struct {
	sync.RWMutex
	Package *PackageConfig
	Assets  *AssetsConfig
}

type PackageConfig struct {
	Name         string
	Version      string
	Authors      []string
	Dependencies []string `toml:"deps"`
	BuildOpts    []string `toml:"build_opts"`
	OmitTests    []string `toml:"omit_tests"`
}

type AssetsConfig struct {
	Dependencies []string `toml:"deps"`
}

func usage() {
	fmt.Println("Usage:")
	fmt.Println("  run       Will watch your file changes and run the application")
	fmt.Println("  dist      Build your web application")
	os.Exit(1)
}

func main() {
	fmt.Println(gocolorize.NewColor("magenta").Paint("gobuildweb > Build a Golang web application.\n"))

	cmds := map[string]Command{
		"run":  commandRun,
		"dist": commandDist,
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
		SUCC.Printf("Loaded project.toml... %s", rootConfig.Package.Name)

		if err := cmd(args[1:]); err != nil {
			ERROR.Fatalf("Executing command [%v] error, %v", args[0], err)
		}
	}
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
	INFO  *log.Logger
	SUCC  *log.Logger
	WARN  *log.Logger
	ERROR *log.Logger
)

func init() {
	INFO = log.New(os.Stdout, "[INFO] ", log.LstdFlags)
	SUCC = log.New(&ColoredLogger{gocolorize.NewColor("green"), os.Stdout}, "[SUCC] ", log.LstdFlags)
	WARN = log.New(&ColoredLogger{gocolorize.NewColor("yellow"), os.Stdout}, "[WARN] ", log.LstdFlags)
	ERROR = log.New(&ColoredLogger{gocolorize.NewColor("red"), os.Stdout}, "[ERROR] ", log.LstdFlags)

	runtime.GOMAXPROCS(runtime.NumCPU())
}

package main

import (
	"flag"
	"fmt"
	"os"
	"runtime"
	"sync"

	"github.com/BurntSushi/toml"
	"github.com/agtorre/gocolorize"
	"github.com/mijia/gobuildweb/assets"
	"github.com/mijia/gobuildweb/loggers"
)

type ProjectConfig struct {
	sync.RWMutex
	Package      *PackageConfig
	Assets       *assets.Config
	Distribution *DistributionConfig
}

func (pc ProjectConfig) getAssetEntry(entryName string) (assets.Entry, bool) {
	pc.RLock()
	defer pc.RUnlock()
	return assets.GetEntryConfig(*pc.Assets, entryName)
}

type PackageConfig struct {
	Name         string
	Version      string
	Authors      []string
	Dependencies []string `toml:"deps"`
	BuildOpts    []string `toml:"build_opts"`
	OmitTests    []string `toml:"omit_tests"`
	IsGraceful   bool     `toml:"is_graceful"`
}

type DistributionConfig struct {
	BuildOpts    []string    `toml:"build_opts"`
	CrossTargets [][2]string `toml:"cross_targets"`
}

func usage() {
	fmt.Println("Usage:")
	fmt.Println("  run       Will watch your file changes and run the application")
	fmt.Println("  dist      Build your web application")
	os.Exit(1)
}

func main() {
	loggers.IsDebug = os.Getenv("GBW_DEBUG") == "1"
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
			loggers.ERROR.Fatalf("Please provide a project.toml for web project.")
		} else if err != nil {
			loggers.ERROR.Fatalf("Accessing project.toml file error, %v.", err)
		} else if fi.IsDir() {
			loggers.ERROR.Fatalf("project.toml cannot be a directory.")
		}

		if _, err := toml.DecodeFile("project.toml", &rootConfig); err != nil {
			loggers.ERROR.Fatalf("Cannot decode the project.toml into TOML format, %v", err)
		}
		loggers.SUCC.Printf("Loaded project.toml... %s", rootConfig.Package.Name)
		if err := cmd(args[1:]); err != nil {
			loggers.ERROR.Fatalf("Executing command [%v] error, %v", args[0], err)
		}
	}
}

var rootConfig ProjectConfig

func init() {
	runtime.GOMAXPROCS(runtime.NumCPU())
}

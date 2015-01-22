package main

import (
	"fmt"
	"os"
	"os/exec"
	"runtime"
)

type TaskType int

const (
	kTaskBuildBinary TaskType = iota
	kTaskBuildAssets
	kTaskRestart
	kTaskKill
	kTaskBinaryTest
)

type AppShellTask struct {
	taskType TaskType
	module   string
}

type AppShell struct {
	args     []string
	taskChan chan AppShellTask
	curError error
	cmd      *exec.Cmd
}

func (app *AppShell) Run() error {
	go app.startRunner()
	app.taskChan <- AppShellTask{kTaskBuildAssets, "."}
	app.taskChan <- AppShellTask{kTaskBinaryTest, "."}
	app.taskChan <- AppShellTask{kTaskBuildBinary, ""}
	app.taskChan <- AppShellTask{kTaskRestart, ""}
	return nil
}

func (app *AppShell) Dist() error {
	fmt.Println()
	INFO.Printf("Creating distribution package for %v-%v",
		rootConfig.Package.Name, rootConfig.Package.Version)
	return nil
}

func (app *AppShell) startRunner() {
	for task := range app.taskChan {
		switch task.taskType {
		case kTaskBuildAssets:
			app.curError = app.buildAssets(task.module)
		case kTaskBuildBinary:
			app.curError = app.buildBinary()
		case kTaskRestart:
			if app.curError == nil {
				INFO.Println("Will restart the server")
			} else {
				WARN.Printf("You have errors with current assets and binary, please fix that ...")
			}
		}
	}
}

func (app *AppShell) buildAssets(module string) error {
	return nil
}

func (app *AppShell) buildBinary() error {
	if goPath, err := exec.LookPath("go"); err != nil {
		ERROR.Println("Cannot find the go executable in PATH.")
		return err
	} else {
		goOs, goArch := runtime.GOOS, runtime.GOARCH
		if goosEnv := os.Getenv("GOOS"); goosEnv != "" {
			goOs = goosEnv
		}
		if goarchEnv := os.Getenv("GOARCH"); goarchEnv != "" {
			goArch = goarchEnv
		}

		rootConfig.RLock()
		binName := fmt.Sprintf("%s-%s.%s.%s", rootConfig.Package.Name, rootConfig.Package.Version,
			goOs, goArch)
		buildOpts := rootConfig.Package.BuildOpts
		rootConfig.RUnlock()

		if goOs == "windows" {
			binName += ".exe"
		}
		flags := make([]string, 0, 3+len(buildOpts))
		flags = append(flags, "build")
		flags = append(flags, buildOpts...)
		flags = append(flags, []string{"-o", binName}...)
		buildCmd := exec.Command(goPath, flags...)
		INFO.Println("Running build:", buildCmd.Args)
		if output, err := buildCmd.CombinedOutput(); err != nil {
			ERROR.Println("Building failed:", string(output))
			return err
		}
	}
	return nil
}

func NewAppShell(args []string) *AppShell {
	return &AppShell{
		args:     args,
		taskChan: make(chan AppShellTask),
	}
}

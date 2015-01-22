package main

import (
	"fmt"
	"os"
	"os/exec"
	"runtime"
	"time"
)

type TaskType int

const (
	// The Order is important
	kTaskBuildSprite TaskType = iota
	kTaskBuildAssets
	kTaskBinaryTest
	kTaskBuildBinary
	kTaskBinaryRestart
)

type AppShellTask struct {
	taskType TaskType
	module   string
}

type AppShell struct {
	binName  string
	args     []string
	taskChan chan AppShellTask
	curError error
	command  *exec.Cmd
}

func (app *AppShell) Run() error {
	go app.startRunner()
	app.executeTask(
		AppShellTask{kTaskBuildSprite, "."},
		AppShellTask{kTaskBuildAssets, "."},
		AppShellTask{kTaskBuildBinary, ""},
	)
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
		case kTaskBuildSprite:
			app.curError = app.buildSprites(task.module)
		case kTaskBuildAssets:
			app.curError = app.buildAssets(task.module)
		case kTaskBinaryTest:
			app.curError = app.binaryTest(task.module)
		case kTaskBuildBinary:
			app.curError = app.buildBinary()
		case kTaskBinaryRestart:
			if app.curError == nil {
				if err := app.kill(); err != nil {
					ERROR.Println("App cannot be killed, maybe you should restart the gobuildweb:", err)
				} else {
					if err := app.start(); err != nil {
						ERROR.Println("App cannot be started, maybe you should restart the gobuildweb:", err)
					}
				}
			} else {
				WARN.Printf("You have errors with current assets and binary, please fix that ...")
			}
			fmt.Println()
			INFO.Println("Waiting for the file changes ...")
		}
	}
}

func (app *AppShell) executeTask(tasks ...AppShellTask) {
	for _, task := range tasks {
		app.taskChan <- task
	}
	app.taskChan <- AppShellTask{kTaskBinaryRestart, ""}
}

func (app *AppShell) kill() error {
	if app.command != nil && (app.command.ProcessState == nil || !app.command.ProcessState.Exited()) {
		if runtime.GOOS == "windows" {
			if err := app.command.Process.Kill(); err != nil {
				return err
			}
		} else if err := app.command.Process.Signal(os.Interrupt); err != nil {
			return err
		}
		//Wait for our process to die before we return or hard kill after 3 sec
		select {
		case <-time.After(3 * time.Second):
			if err := app.command.Process.Kill(); err != nil {
				WARN.Println("failed to kill the app: ", err)
			}
		}
		app.command = nil
	}
	return nil
}

func (app *AppShell) start() error {
	app.command = exec.Command("./"+app.binName, app.args...)
	app.command.Stdout = os.Stdout
	app.command.Stderr = os.Stderr

	if err := app.command.Start(); err != nil {
		return err
	}
	SUCC.Printf("App is starting, %v", app.command.Args)
	fmt.Println()
	go app.command.Wait()
	time.Sleep(500 * time.Millisecond)
	return nil
}

func (app *AppShell) buildSprites(module string) error {
	return nil
}

func (app *AppShell) buildAssets(module string) error {
	return nil
}

func (app *AppShell) binaryTest(module string) error {
	testCmd := exec.Command("go", "test", "-v", module)
	INFO.Printf("Testing Module[%s]: %v", module, testCmd.Args)
	testCmd.Stderr = os.Stderr
	testCmd.Stdout = os.Stdout
	if err := testCmd.Run(); err != nil {
		ERROR.Printf("Error when testing go modules[%s], %v", module, err)
		return err
	}
	return nil
}

func (app *AppShell) buildBinary() error {
	goOs, goArch := runtime.GOOS, runtime.GOARCH
	if goosEnv := os.Getenv("GOOS"); goosEnv != "" {
		goOs = goosEnv
	}
	if goarchEnv := os.Getenv("GOARCH"); goarchEnv != "" {
		goArch = goarchEnv
	}

	buildOpts := make([]string, 0)
	rootConfig.RLock()
	binName := fmt.Sprintf("%s-%s.%s.%s",
		rootConfig.Package.Name, rootConfig.Package.Version,
		goOs, goArch)
	copy(rootConfig.Package.BuildOpts, buildOpts)
	rootConfig.RUnlock()

	if goOs == "windows" {
		binName += ".exe"
	}
	flags := make([]string, 0, 3+len(buildOpts))
	flags = append(flags, "build")
	flags = append(flags, buildOpts...)
	flags = append(flags, []string{"-o", binName}...)
	buildCmd := exec.Command("go", flags...)
	INFO.Println("Running build:", buildCmd.Args)
	start := time.Now()
	if output, err := buildCmd.CombinedOutput(); err != nil {
		ERROR.Println("Building failed:", string(output))
		return err
	}
	app.binName = binName
	duration := float64(time.Since(start).Nanoseconds()) / 1e6
	SUCC.Printf("Got binary built %s, takes=%.3fms", binName, duration)
	return nil
}

func NewAppShell(args []string) *AppShell {
	return &AppShell{
		args:     args,
		taskChan: make(chan AppShellTask),
	}
}

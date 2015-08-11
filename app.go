package main

import (
	"archive/zip"
	"fmt"
	"io"
	"os"
	"os/exec"
	"path"
	"path/filepath"
	"runtime"
	"time"

	"github.com/mijia/gobuildweb/assets"
	"github.com/mijia/gobuildweb/loggers"
)

type TaskType int

const (
	// The Order is important
	kTaskBuildImages TaskType = iota
	kTaskBuildStyles
	kTaskBuildJavaScripts
	kTaskGenAssetsMapping
	kTaskBinaryTest
	kTaskBuildBinary
	kTaskBinaryRestart
)

type AppShellTask struct {
	taskType TaskType
	module   string
}

type AppShell struct {
	binName      string
	args         []string
	isProduction bool
	taskChan     chan AppShellTask
	curError     error
	command      *exec.Cmd
}

func (app *AppShell) Run() error {
	app.isProduction = false
	go app.startRunner()
	app.executeTask(
		AppShellTask{kTaskBuildImages, ""},
		AppShellTask{kTaskBuildStyles, ""},
		AppShellTask{kTaskBuildJavaScripts, ""},
		AppShellTask{kTaskGenAssetsMapping, ""},
		AppShellTask{kTaskBuildBinary, ""},
		AppShellTask{kTaskBinaryRestart, ""},
	)
	return nil
}

func (app *AppShell) Dist() error {
	app.isProduction = true
	fmt.Println()
	loggers.Info("Creating distribution package for %v-%v",
		rootConfig.Package.Name, rootConfig.Package.Version)

	var err error
	if err = app.buildImages(""); err != nil {
		loggers.Error("Error when building images, %v", err)
	} else if err = app.buildStyles(""); err != nil {
		loggers.Error("Error when building stylesheets, %v", err)
	} else if err = app.buildJavaScripts(""); err != nil {
		loggers.Error("Error when building javascripts, %v", err)
	} else if err = app.genAssetsMapping(); err != nil {
		loggers.Error("Error when generating assets mapping source code, %v", err)
	} else if err = app.binaryTest(""); err != nil {
		loggers.Error("You have failed test cases, %v", err)
	} else if err = app.distExtraCommand(); err != nil {
		loggers.Error("Error when running the distribution extra command, %v", err)
	} else if err == nil {
		goOs, goArch := runtime.GOOS, runtime.GOARCH
		targets := append(rootConfig.Distribution.CrossTargets, [2]string{goOs, goArch})
		visited := make(map[string]struct{})
		for _, target := range targets {
			buildTarget := fmt.Sprintf("%s_%s", target[0], target[1])
			if _, ok := visited[buildTarget]; ok {
				continue
			}
			visited[buildTarget] = struct{}{}
			if err = app.buildBinary(target[:]...); err != nil {
				loggers.Error("Error when building binary for %v, %v", target, err)
			}
		}
	}
	if err == nil {
		err = app.buildPackage()
	}
	return err
}

func (app *AppShell) distExtraCommand() error {
	extraCmd := rootConfig.Distribution.ExtraCmd
	if extraCmd == nil || len(extraCmd) == 0 {
		return nil
	}
	cmd := exec.Command(extraCmd[0], extraCmd[1:]...)
	cmd.Stderr = os.Stderr
	cmd.Stdout = os.Stdout
	if err := cmd.Run(); err != nil {
		loggers.Error("Error when running distribution extra command, %v, %s", extraCmd, err)
		return err
	}
	loggers.Succ("Run extra command succ: %v", cmd.Args)
	return nil
}

func (app *AppShell) buildPackage() error {
	name, version := rootConfig.Package.Name, rootConfig.Package.Version
	pkgName := fmt.Sprintf("%s-%s", name, version)
	srcFolders := append([]string{"public"}, rootConfig.Distribution.PackExtras...)

	goOs, goArch := runtime.GOOS, runtime.GOARCH
	targets := append(rootConfig.Distribution.CrossTargets, [2]string{goOs, goArch})
	visited := make(map[string]struct{})
	for _, target := range targets {
		buildTarget := fmt.Sprintf("%s_%s", target[0], target[1])
		if _, ok := visited[buildTarget]; ok {
			continue
		}
		visited[buildTarget] = struct{}{}
		srcFolders = append(srcFolders, app.binaryName(name, version, target[0], target[1]))
	}

	if zipFile, err := os.Create(pkgName + ".zip"); err != nil {
		return fmt.Errorf("Cannot create the zip file[%q], %v", pkgName, err)
	} else {
		defer zipFile.Close()
		zw := zip.NewWriter(zipFile)
		defer zw.Close()
		for _, srcFolder := range srcFolders {
			err := filepath.Walk(srcFolder, func(fn string, info os.FileInfo, err error) error {
				if err == nil && !info.IsDir() {
					zipSrcName := path.Join(pkgName, fn)
					fileHeader, err := zip.FileInfoHeader(info)
					if err != nil {
						return err
					}
					fileHeader.Name = zipSrcName
					zipSrcFile, err := zw.CreateHeader(fileHeader)
					if err != nil {
						return err
					}
					srcFile, err := os.Open(fn)
					if err != nil {
						return err
					}
					io.Copy(zipSrcFile, srcFile)
					srcFile.Close()
					loggers.Debug("Archiving %s", zipSrcName)
				}
				return nil
			})
			if err != nil {
				return fmt.Errorf("Cannot walk the files when creating the zip file, %v", err)
			}
		}
	}

	loggers.Succ("Finish packing the deploy package in %s.zip", pkgName)
	return nil
}

func (app *AppShell) startRunner() {
	for task := range app.taskChan {
		switch task.taskType {
		case kTaskBuildImages:
			app.curError = app.buildImages(task.module)
		case kTaskBuildStyles:
			app.curError = app.buildStyles(task.module)
		case kTaskBuildJavaScripts:
			app.curError = app.buildJavaScripts(task.module)
		case kTaskGenAssetsMapping:
			app.curError = app.genAssetsMapping()
		case kTaskBinaryTest:
			app.curError = app.binaryTest(task.module)
		case kTaskBuildBinary:
			app.curError = app.buildBinary()
		case kTaskBinaryRestart:
			if app.curError == nil {
				if err := app.kill(); err != nil {
					loggers.Error("App cannot be killed, maybe you should restart the gobuildweb: %v", err)
				} else {
					if err := app.start(); err != nil {
						loggers.Error("App cannot be started, maybe you should restart the gobuildweb: %v", err)
					}
				}
			} else {
				loggers.Warn("You have errors with current assets and binary, please fix that ...")
			}
			fmt.Println()
			loggers.Info("Waiting for the file changes ...")
		}
	}
}

func (app *AppShell) executeTask(tasks ...AppShellTask) {
	for _, task := range tasks {
		app.taskChan <- task
	}
	//app.taskChan <- AppShellTask{kTaskBinaryRestart, ""}
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

		rootConfig.RLock()
		isGraceful := rootConfig.Package.IsGraceful
		rootConfig.RUnlock()

		if !isGraceful {
			// Wait for our process to die before we return or hard kill after 3 sec
			// when this is not a graceful server
			select {
			case <-time.After(3 * time.Second):
				if err := app.command.Process.Kill(); err != nil {
					loggers.Warn("failed to kill the app: %v", err)
				}
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
	app.command.Env = mergeEnv(nil)

	if err := app.command.Start(); err != nil {
		return err
	}
	loggers.Succ("App is starting, %v", app.command.Args)
	fmt.Println()
	go app.command.Wait()
	time.Sleep(500 * time.Millisecond)
	return nil
}

func (app *AppShell) buildAssetsTraverse(functor func(entry string) error) error {
	rootConfig.RLock()
	vendors := rootConfig.Assets.VendorSets
	entries := rootConfig.Assets.Entries
	rootConfig.RUnlock()
	for _, vendor := range vendors {
		if err := functor(vendor.Name); err != nil {
			return err
		}
	}
	for _, entry := range entries {
		if err := functor(entry.Name); err != nil {
			return err
		}
	}
	return nil
}

func (app *AppShell) buildImages(entry string) error {
	rootConfig.RLock()
	if rootConfig.Assets == nil {
		rootConfig.RUnlock()
		return nil
	}
	rootConfig.RUnlock()

	if entry == "" {
		if err := assets.ResetDir("public/images", true); err != nil {
			return err
		}
		return app.buildAssetsTraverse(app.buildImages)
	}

	rootConfig.RLock()
	defer rootConfig.RUnlock()
	return assets.ImageLibrary(*rootConfig.Assets, entry).Build(app.isProduction)
}

func (app *AppShell) buildStyles(entry string) error {
	rootConfig.RLock()
	if rootConfig.Assets == nil {
		rootConfig.RUnlock()
		return nil
	}
	rootConfig.RUnlock()

	if entry == "" {
		if err := assets.ResetDir("public/stylesheets", true); err != nil {
			return err
		}
		return app.buildAssetsTraverse(app.buildStyles)
	}

	rootConfig.RLock()
	defer rootConfig.RUnlock()
	return assets.StyleSheet(*rootConfig.Assets, entry).Build(app.isProduction)
}

func (app *AppShell) buildJavaScripts(entry string) error {
	rootConfig.RLock()
	if rootConfig.Assets == nil {
		rootConfig.RUnlock()
		return nil
	}
	rootConfig.RUnlock()

	if entry == "" {
		if err := assets.ResetDir("public/javascripts", true); err != nil {
			return err
		}
		return app.buildAssetsTraverse(app.buildJavaScripts)
	}

	rootConfig.RLock()
	defer rootConfig.RUnlock()
	return assets.JavaScript(*rootConfig.Assets, entry).Build(app.isProduction)
}

func (app *AppShell) genAssetsMapping() error {
	rootConfig.RLock()
	defer rootConfig.RUnlock()
	if rootConfig.Assets == nil {
		return nil
	}
	return assets.Mappings(*rootConfig.Assets).Build(app.isProduction)
}

func (app *AppShell) binaryTest(module string) error {
	return nil // close the test first, will reconsider this
	if module == "" {
		module = "./..."
	}
	testCmd := exec.Command("go", "test", "-v", module)
	testCmd.Stderr = os.Stderr
	testCmd.Stdout = os.Stdout
	testCmd.Env = mergeEnv(nil)
	if err := testCmd.Run(); err != nil {
		loggers.Error("Error when testing go modules[%s], %v", module, err)
		return err
	}
	loggers.Succ("Module[%s] Test passed: %v", module, testCmd.Args)
	return nil
}

func (app *AppShell) binaryName(name, version, goOs, goArch string) string {
	binName := fmt.Sprintf("%s-%s.%s.%s", name, version, goOs, goArch)
	if goOs == "windows" {
		binName += ".exe"
	}
	return binName
}

func (app *AppShell) buildBinary(params ...string) error {
	goOs, goArch := runtime.GOOS, runtime.GOARCH
	if len(params) == 2 && (goOs != params[0] || goArch != params[1]) {
		goOs, goArch = params[0], params[1]
	}

	rootConfig.RLock()
	binName := app.binaryName(rootConfig.Package.Name, rootConfig.Package.Version, goOs, goArch)
	var buildOpts []string
	if app.isProduction {
		buildOpts = make([]string, len(rootConfig.Distribution.BuildOpts))
		copy(buildOpts, rootConfig.Distribution.BuildOpts)
	} else {
		buildOpts = make([]string, len(rootConfig.Package.BuildOpts))
		copy(buildOpts, rootConfig.Package.BuildOpts)
	}
	rootConfig.RUnlock()

	flags := make([]string, 0, 3+len(buildOpts))
	flags = append(flags, "build")
	flags = append(flags, buildOpts...)
	flags = append(flags, []string{"-o", binName}...)
	buildCmd := exec.Command("go", flags...)
	buildCmd.Stderr = os.Stderr
	buildCmd.Stdout = os.Stdout
	buildCmd.Env = mergeEnv(map[string]string{
		"GOOS":   goOs,
		"GOARCH": goArch,
	})
	loggers.Debug("Running build: %v", buildCmd.Args)
	start := time.Now()
	if err := buildCmd.Run(); err != nil {
		loggers.Error("Building failed")
		return err
	}
	app.binName = binName
	duration := float64(time.Since(start).Nanoseconds()) / 1e6
	loggers.Succ("Got binary built %s, takes=%.3fms", binName, duration)
	return nil
}

func NewAppShell(args []string) *AppShell {
	return &AppShell{
		args:     args,
		taskChan: make(chan AppShellTask),
	}
}

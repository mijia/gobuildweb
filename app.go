package main

import (
	"crypto/md5"
	"encoding/hex"
	"fmt"
	"io"
	"os"
	"os/exec"
	"path"
	"path/filepath"
	"runtime"
	"strings"
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
		AppShellTask{kTaskBuildBinary, ""},
	)
	return nil
}

func (app *AppShell) Dist() error {
	app.isProduction = true
	fmt.Println()
	loggers.INFO.Printf("Creating distribution package for %v-%v",
		rootConfig.Package.Name, rootConfig.Package.Version)

	var err error
	if err = app.buildImages(""); err != nil {
		loggers.ERROR.Printf("Error when building images, %v", err)
	} else if err = app.buildStyles(""); err != nil {
		loggers.ERROR.Printf("Error when building stylesheets, %v", err)
	} else if err = app.buildJavaScripts(""); err != nil {
		loggers.ERROR.Printf("Error when building javascripts, %v", err)
	} else if err = app.binaryTest(""); err != nil {
		loggers.ERROR.Printf("You have failed test cases, %v", err)
	} else if err == nil {
		for _, target := range rootConfig.Distribution.CrossTargets {
			if err = app.buildBinary(target...); err != nil {
				loggers.ERROR.Printf("Error when building binary for %v, %v", target, err)
			}
		}
	}
	// TODO package all the binary and static assets
	return err
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
		case kTaskBinaryTest:
			app.curError = app.binaryTest(task.module)
		case kTaskBuildBinary:
			app.curError = app.buildBinary()
		case kTaskBinaryRestart:
			if app.curError == nil {
				if err := app.kill(); err != nil {
					loggers.ERROR.Println("App cannot be killed, maybe you should restart the gobuildweb:", err)
				} else {
					if err := app.start(); err != nil {
						loggers.ERROR.Println("App cannot be started, maybe you should restart the gobuildweb:", err)
					}
				}
			} else {
				loggers.WARN.Printf("You have errors with current assets and binary, please fix that ...")
			}
			fmt.Println()
			loggers.INFO.Println("Waiting for the file changes ...")
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
				loggers.WARN.Println("failed to kill the app: ", err)
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
	loggers.SUCC.Printf("App is starting, %v", app.command.Args)
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

func (app *AppShell) checkAssetEntry(filename string, needsFile bool) (proceed bool, err error) {
	if fi, err := os.Stat(filename); os.IsNotExist(err) {
		return false, nil
	} else if err != nil {
		return false, err
	} else if fi.IsDir() && needsFile {
		return false, fmt.Errorf("%s is a directory!", filename)
	}
	return true, nil
}

func (app *AppShell) removeAssetFile(dir string, suffix string) error {
	return filepath.Walk(dir, func(fname string, info os.FileInfo, err error) error {
		if err == nil && !info.IsDir() && strings.HasSuffix(fname, suffix) {
			filename := info.Name()
			if strings.HasPrefix(filename, "fp") && strings.HasSuffix(filename, "-"+suffix) {
				os.Remove(fname)
			}
		}
		if fname != dir && info.IsDir() {
			return filepath.SkipDir
		}
		return nil
	})
}

func (app AppShell) copyAssetFile(dest, src string) error {
	if srcFile, err := os.Open(src); err != nil {
		return err
	} else {
		defer srcFile.Close()
		if destFile, err := os.Create(dest); err != nil {
			return err
		} else {
			defer destFile.Close()
			_, err := io.Copy(destFile, srcFile)
			return err
		}
	}
}

func (app *AppShell) addFingerPrint(assetDir, filename string) string {
	target := path.Join(assetDir, filename)
	if file, err := os.Open(target); err == nil {
		defer file.Close()
		h := md5.New()
		if _, err := io.Copy(h, file); err == nil {
			newName := fmt.Sprintf("fp%s-%s", hex.EncodeToString(h.Sum(nil)), filename)
			newName = path.Join(assetDir, newName)
			app.removeAssetFile(assetDir, filename)
			if err := os.Rename(target, newName); err == nil {
				return newName
			}
		}
	}
	return target
}

func (app *AppShell) resetAssetsDir(dir string, rebuild bool) error {
	if err := os.RemoveAll(dir); err != nil {
		return fmt.Errorf("Cannot clean %s, %v", dir, err)
	}
	if rebuild {
		if err := os.MkdirAll(dir, os.ModePerm|os.ModeDir); err != nil {
			return fmt.Errorf("Cannot mkdir %s, %v", dir, err)
		}
	}
	return nil
}

func (app *AppShell) getImageAssetsList(folderName string) ([][]string, error) {
	allowedExts := make(map[string]struct{})
	rootConfig.RLock()
	for _, ext := range rootConfig.Assets.ImageExts {
		allowedExts[ext] = struct{}{}
	}
	rootConfig.RUnlock()
	imagesPath := make([][]string, 0)
	err := filepath.Walk(folderName, func(fname string, info os.FileInfo, err error) error {
		if err == nil && !info.IsDir() {
			if _, ok := allowedExts[filepath.Ext(fname)]; ok {
				imagesPath = append(imagesPath, []string{fname, info.Name()})
			}
		}
		if fname != folderName && info.IsDir() {
			return filepath.SkipDir
		}
		return nil
	})
	return imagesPath, err
}

func (app *AppShell) buildImages(entry string) error {
	if entry == "" {
		if err := app.resetAssetsDir("public/images", true); err != nil {
			return err
		}
		return app.buildAssetsTraverse(app.buildImages)
	}

	return assets.ImageLibrary(*rootConfig.Assets, entry).Build(app.isProduction)
}

func (app *AppShell) buildStyles(entry string) error {
	if entry == "" {
		if err := assets.ResetDir("public/stylesheets", true); err != nil {
			return err
		}
		return app.buildAssetsTraverse(app.buildStyles)
	}

	return assets.StyleSheet(*rootConfig.Assets, entry).Build(app.isProduction)
}

func (app *AppShell) buildJavaScripts(entry string) error {
	if entry == "" {
		if err := assets.ResetDir("public/javascripts", true); err != nil {
			return err
		}
		return app.buildAssetsTraverse(app.buildJavaScripts)
	}

	return assets.JavaScript(*rootConfig.Assets, entry).Build(app.isProduction)
}

func (app *AppShell) binaryTest(module string) error {
	if module == "" {
		module = "."
	}
	testCmd := exec.Command("go", "test", module)
	loggers.INFO.Printf("Testing Module[%s]: %v", module, testCmd.Args)
	testCmd.Stderr = os.Stderr
	testCmd.Stdout = os.Stdout
	if err := testCmd.Run(); err != nil {
		loggers.ERROR.Printf("Error when testing go modules[%s], %v", module, err)
		return err
	}
	return nil
}

func (app *AppShell) buildBinary(params ...string) error {
	goOs, goArch := runtime.GOOS, runtime.GOARCH
	if len(params) == 2 && (goOs != params[0] || goArch != params[1]) {
		goOs, goArch = params[0], params[1]
	}

	rootConfig.RLock()
	binName := fmt.Sprintf("%s-%s.%s.%s",
		rootConfig.Package.Name, rootConfig.Package.Version,
		goOs, goArch)
	var buildOpts []string
	if app.isProduction {
		buildOpts = make([]string, len(rootConfig.Distribution.BuildOpts))
		copy(buildOpts, rootConfig.Distribution.BuildOpts)
	} else {
		buildOpts = make([]string, len(rootConfig.Package.BuildOpts))
		copy(buildOpts, rootConfig.Package.BuildOpts)
	}
	rootConfig.RUnlock()

	env := []string{
		fmt.Sprintf("PATH=%s", os.Getenv("PATH")),
		fmt.Sprintf("GOOS=%s", goOs),
		fmt.Sprintf("GOARCH=%s", goArch),
		fmt.Sprintf("GOPATH=%s", os.Getenv("GOPATH")),
	}
	if goOs == "windows" {
		binName += ".exe"
	}

	flags := make([]string, 0, 3+len(buildOpts))
	flags = append(flags, "build")
	flags = append(flags, buildOpts...)
	flags = append(flags, []string{"-o", binName}...)
	buildCmd := exec.Command("go", flags...)
	buildCmd.Env = env
	loggers.INFO.Println("Running build:", buildCmd.Args)
	start := time.Now()
	if output, err := buildCmd.CombinedOutput(); err != nil {
		loggers.ERROR.Println("Building failed:", string(output))
		return err
	}
	app.binName = binName
	duration := float64(time.Since(start).Nanoseconds()) / 1e6
	loggers.SUCC.Printf("Got binary built %s, takes=%.3fms", binName, duration)
	return nil
}

func NewAppShell(args []string) *AppShell {
	return &AppShell{
		args:     args,
		taskChan: make(chan AppShellTask),
	}
}

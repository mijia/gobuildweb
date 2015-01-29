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
	INFO.Printf("Creating distribution package for %v-%v",
		rootConfig.Package.Name, rootConfig.Package.Version)

	var err error
	if err = app.buildImages(""); err != nil {
		ERROR.Printf("Error when building images, %v", err)
	} else if err = app.buildStyles(""); err != nil {
		ERROR.Printf("Error when building stylesheets, %v", err)
	} else if err = app.buildJavaScripts(""); err != nil {
		ERROR.Printf("Error when building javascripts, %v", err)
	} else if err = app.binaryTest(""); err != nil {
		ERROR.Printf("You have failed test cases, %v", err)
	} else if err == nil {
		for _, target := range rootConfig.Distribution.CrossTargets {
			if err = app.buildBinary(target...); err != nil {
				ERROR.Printf("Error when building binary for %v, %v", target, err)
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

func (app *AppShell) checkAssetEntry(filename string) (proceed bool, err error) {
	if fi, err := os.Stat(filename); os.IsNotExist(err) {
		return false, nil
	} else if err != nil {
		return false, err
	} else if fi.IsDir() {
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
			app.removeAssetFile("public/javascripts", filename)
			if err := os.Rename(target, newName); err == nil {
				return newName
			}
		}
	}
	return target
}

func (app *AppShell) resetAssetsDir(dir string) error {
	if err := os.RemoveAll(dir); err != nil {
		return fmt.Errorf("Cannot clean %s, %v", dir, err)
	}
	if err := os.MkdirAll(dir, os.ModePerm|os.ModeDir); err != nil {
		return fmt.Errorf("Cannot mkdir %s, %v", dir, err)
	}
	return nil
}

func (app *AppShell) buildImages(entry string) error {
	if entry == "" {
		if err := app.resetAssetsDir("public/images"); err != nil {
			return err
		}
		return app.buildAssetsTraverse(app.buildImages)
	}
	return nil
}

func (app *AppShell) buildStyles(entry string) error {
	if entry == "" {
		if err := app.resetAssetsDir("public/stylesheets"); err != nil {
			return err
		}
		return app.buildAssetsTraverse(app.buildStyles)
	}

	filename := fmt.Sprintf("assets/stylesheets/%s.styl", entry)
	isStylus := true
	if proceed, _ := app.checkAssetEntry(filename); !proceed {
		filename = fmt.Sprintf("assets/stylesheets/%s.css", entry)
		if proceed, err := app.checkAssetEntry(filename); !proceed {
			return err
		} else {
			isStylus = false
		}
	}

	target := fmt.Sprintf("public/stylesheets/%s.css", entry)
	// * Maybe it's a template using images, styles assets links
	// TODO

	// Maybe we need to call stylus preprocess
	if isStylus {

	} else {
		if err := app.copyAssetFile(target, filename); err != nil {
			return err
		}
	}

	// * generate the hash, clear old bundle, move to target
	target = app.addFingerPrint("public/stylesheets", entry+".css")
	SUCC.Printf("Saved stylesheet asssets[%s]: %s", entry, target)

	return nil
}

func (app *AppShell) buildJavaScripts(entry string) error {
	if entry == "" {
		if err := app.resetAssetsDir("public/javascripts"); err != nil {
			return err
		}
		return app.buildAssetsTraverse(app.buildJavaScripts)
	}

	filename := fmt.Sprintf("assets/javascripts/%s.js", entry)
	target := fmt.Sprintf("public/javascripts/%s.js", entry)
	if proceed, err := app.checkAssetEntry(filename); !proceed {
		return err
	}

	// * Maybe it's a template using images, styles assets links
	// TODO

	// * run browserify
	assetEntry, ok := rootConfig.getAssetEntry(entry)
	if !ok {
		return nil
	}
	params := make([]string, 0)
	params = append(params, filename)
	for _, require := range assetEntry.Requires {
		params = append(params, "--require", require)
	}
	for _, external := range assetEntry.Externals {
		if anEntry, ok := rootConfig.getAssetEntry(external); ok {
			for _, require := range anEntry.Requires {
				params = append(params, "--external", require)
			}
		}
	}
	for _, opt := range assetEntry.BundleOpts {
		params = append(params, opt)
	}
	params = append(params, "--transform", "reactify")
	params = append(params, "--transform", "envify")
	if !app.isProduction {
		params = append(params, "--debug")
	} else {
		params = append(params, "-g", "uglifyify")
	}

	params = append(params, "--outfile", target)
	bfyCmd := exec.Command("./node_modules/browserify/bin/cmd.js", params...)
	INFO.Printf("Building JavaScript assets: %s, %v", filename, bfyCmd.Args)
	bfyCmd.Stderr = os.Stderr
	bfyCmd.Stdout = os.Stdout
	bfyCmd.Env = []string{
		fmt.Sprintf("NODE_PATH=%s:./node_modules", os.Getenv("NODE_PATH")),
		fmt.Sprintf("PATH=%s", os.Getenv("PATH")),
	}
	if app.isProduction {
		bfyCmd.Env = append(bfyCmd.Env, "NODE_ENV=production")
	} else {
		bfyCmd.Env = append(bfyCmd.Env, "NODE_ENV=development")
	}

	if err := bfyCmd.Run(); err != nil {
		ERROR.Printf("Error when building javascript asssets [%v], %v", bfyCmd.Args, err)
		return err
	}

	// * generate the hash, clear old bundle, move to target
	target = app.addFingerPrint("public/javascripts", entry+".js")
	SUCC.Printf("Saved javascript asssets[%s]: %s", entry, target)
	return nil
}

func (app *AppShell) binaryTest(module string) error {
	if module == "" {
		module = "."
	}
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

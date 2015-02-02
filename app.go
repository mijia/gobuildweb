package main

import (
	"crypto/md5"
	"encoding/hex"
	"fmt"
	"image"
	"image/draw"
	_ "image/jpeg"
	"image/png"
	"io"
	"os"
	"os/exec"
	"path"
	"path/filepath"
	"runtime"
	"strings"
	"text/template"
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

	folderName := fmt.Sprintf("assets/images/%s", entry)
	if proceed, err := app.checkAssetEntry(folderName, false); !proceed {
		return err
	}
	targetFolder := fmt.Sprintf("public/images/%s", entry)
	if err := app.resetAssetsDir(targetFolder, true); err != nil {
		return err
	}

	// copy the single image files
	if imagesPath, err := app.getImageAssetsList(folderName); err != nil {
		return err
	} else {
		for _, imgPath := range imagesPath {
			target := fmt.Sprintf("public/images/%s/%s", entry, imgPath[1])
			if err := app.copyAssetFile(target, imgPath[0]); err != nil {
				return err
			}
			target = app.addFingerPrint(targetFolder, imgPath[1])
			SUCC.Printf("Saved images asssets[%s]: %s", entry, target)
		}
	}

	// check if we have a sprites folder under assets
	return app.buildSprite(entry)
}

func (app *AppShell) buildSprite(entry string) error {
	if entry == "" {
		return nil
	}
	folderName := fmt.Sprintf("assets/images/%s/sprites", entry)
	if proceed, err := app.checkAssetEntry(folderName, false); !proceed {
		return err
	}
	targetFolder := fmt.Sprintf("public/images/%s", entry)
	if err := os.MkdirAll(targetFolder, os.ModePerm|os.ModeDir); err != nil {
		return fmt.Errorf("Cannot mkdir %s, %v", targetFolder, err)
	}

	allowedExts := make(map[string]struct{})
	rootConfig.RLock()
	for _, ext := range rootConfig.Assets.ImageExts {
		allowedExts[ext] = struct{}{}
	}
	rootConfig.RUnlock()
	if imagesPath, err := app.getImageAssetsList(folderName); err != nil {
		return err
	} else {
		imageFiles := make([]*os.File, len(imagesPath))
		images := make([]image.Image, len(imagesPath))
		width, height := 0, 0
		for i, imgPath := range imagesPath {
			if fImage, err := os.Open(imgPath[0]); err != nil {
				return fmt.Errorf("Cannot open image file: %s, %v", imgPath[0], err)
			} else {
				if image, _, err := image.Decode(fImage); err != nil {
					return err
				} else {
					imageFiles[i] = fImage
					images[i] = image
					height += image.Bounds().Dy()
					if width < image.Bounds().Dx() {
						width = image.Bounds().Dx()
					}
				}
			}
		}
		spriteImage := image.NewNRGBA(image.Rect(0, 0, width, height))
		yOffset := 0
		for i := range images {
			imageFiles[i].Close()
			newBounds := image.Rect(0, yOffset, images[i].Bounds().Dx(), yOffset+images[i].Bounds().Dy())
			draw.Draw(spriteImage, newBounds, images[i], image.Point{0, 0}, draw.Src)
			yOffset += images[i].Bounds().Dy()
		}
		target := path.Join(targetFolder, "sprites.png")
		if spriteFile, err := os.Create(target); err != nil {
			return err
		} else {
			defer spriteFile.Close()
			if err := png.Encode(spriteFile, spriteImage); err != nil {
				return err
			}
			target = app.addFingerPrint(targetFolder, "sprites.png")
			SUCC.Printf("Saved sprites image[%s]: %s", entry, target)

			spriteStylus := fmt.Sprintf("assets/stylesheets/sprites")
			if err := os.MkdirAll(spriteStylus, os.ModePerm|os.ModeDir); err != nil {
				return fmt.Errorf("Cannot mkdir %s, %v", spriteStylus, err)
			}
			spriteStylus = fmt.Sprintf("assets/stylesheets/sprites/%s.styl", entry)
			if file, err := os.Create(spriteStylus); err != nil {
				return fmt.Errorf("Cannot create the stylus file for sprite, %s, %v", spriteStylus, err)
			} else {
				defer file.Close()
				rootConfig.RLock()
				urlPrefix := rootConfig.Assets.UrlPrefix
				rootConfig.RUnlock()
				spriteEntry := SpriteEntry{
					Entry:   entry,
					Url:     fmt.Sprintf("%s/images/%s/%s", urlPrefix, entry, filepath.Base(target)),
					Sprites: make([]SpriteImage, len(images)),
				}

				pixelRatio := 1
				if assetEntry, ok := rootConfig.getAssetEntry(entry); ok && assetEntry.SpritePixelRatio > 0 {
					pixelRatio = assetEntry.SpritePixelRatio
				}
				lastHeight := 0
				for i, image := range images {
					name := imagesPath[i][1]
					name = name[:len(name)-len(filepath.Ext(name))]
					width, height := image.Bounds().Dx(), image.Bounds().Dy()
					if width%pixelRatio != 0 || height%pixelRatio != 0 {
						WARN.Printf("You have images cannot be adjusted by the pixel ratio, %s, bounds=%v, pixelRatio=%d",
							imagesPath[i][0], images[i].Bounds(), pixelRatio)
					}
					spriteEntry.Sprites[i] = SpriteImage{
						Name:   name,
						X:      0,
						Y:      -1 * lastHeight,
						Width:  width / pixelRatio,
						Height: height / pixelRatio,
					}
					lastHeight = height / pixelRatio
				}
				if err := tmSprites.Execute(file, spriteEntry); err != nil {
					return fmt.Errorf("Cannot generate stylus for sprites, %s, %v", spriteEntry, err)
				}
			}
		}
	}
	return nil
}

func (app *AppShell) buildStyles(entry string) error {
	if entry == "" {
		if err := app.resetAssetsDir("public/stylesheets", true); err != nil {
			return err
		}
		return app.buildAssetsTraverse(app.buildStyles)
	}

	filename := fmt.Sprintf("assets/stylesheets/%s.styl", entry)
	isStylus := true
	if proceed, _ := app.checkAssetEntry(filename, true); !proceed {
		filename = fmt.Sprintf("assets/stylesheets/%s.css", entry)
		if proceed, err := app.checkAssetEntry(filename, true); !proceed {
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
		params := make([]string, 0)
		params = append(params, "--use", "nib", filename, "--out", "public/stylesheets")
		if app.isProduction {
			params = append(params, "--compress")
		} else {
			params = append(params, "--sourcemap-inline")
		}
		stylusCmd := exec.Command("./node_modules/stylus/bin/stylus", params...)
		INFO.Printf("Buidling StyleSheet assets: %s, %v", filename, stylusCmd.Args)
		stylusCmd.Stderr = os.Stderr
		stylusCmd.Stdout = os.Stdout
		stylusCmd.Env = []string{
			fmt.Sprintf("NODE_PATH=%s:./node_modules", os.Getenv("NODE_PATH")),
			fmt.Sprintf("PATH=%s", os.Getenv("PATH")),
		}
		if app.isProduction {
			stylusCmd.Env = append(stylusCmd.Env, "NODE_ENV=production")
		} else {
			stylusCmd.Env = append(stylusCmd.Env, "NODE_ENV=development")
		}
		if err := stylusCmd.Run(); err != nil {
			ERROR.Printf("Error when building stylesheet asssets [%v], %v", stylusCmd.Args, err)
			return err
		}
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
		if err := app.resetAssetsDir("public/javascripts", true); err != nil {
			return err
		}
		return app.buildAssetsTraverse(app.buildJavaScripts)
	}

	filename := fmt.Sprintf("assets/javascripts/%s.js", entry)
	target := fmt.Sprintf("public/javascripts/%s.js", entry)
	if proceed, err := app.checkAssetEntry(filename, true); !proceed {
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
	testCmd := exec.Command("go", "test", module)
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

type SpriteEntry struct {
	Entry   string
	Url     string
	Sprites []SpriteImage
}

type SpriteImage struct {
	Name   string
	X      int
	Y      int
	Width  int
	Height int
}

var tmplSprites = `{{$EntryName := .Entry }}
{{range .Sprites}}${{$EntryName}}-{{.Name}} = {{.X}}px {{.Y}}px {{.Width}}px {{.Height}}px
{{end}}
{{.Entry}}-sprite($sprite)
  background-image url("{{.Url}}")
  background-position $sprite[0] $sprite[1]
  width $sprite[2]
  height $sprite[3]
`
var tmSprites *template.Template

func init() {
	tmSprites = template.Must(template.New("sprites").Parse(tmplSprites))
}

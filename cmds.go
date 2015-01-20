package main

import (
	"fmt"
	"os"
	"os/exec"
	"path"
	"path/filepath"
	"strings"

	"gopkg.in/fsnotify.v1"
)

type Command func(args []string) error

func commandDist(args []string) error {
	if err := updateGolangDeps(); err != nil {
		ERROR.Printf("Failed to load project #Golang dependencies, %v", err)
		return err
	}
	if err := updateAssetsDeps(); err != nil {
		ERROR.Printf("Failed to load project assets dependencies, %v", err)
		return err
	}

	return NewAppShell(args).Dist()
}

func commandRun(args []string) error {
	if err := updateGolangDeps(); err != nil {
		ERROR.Printf("Failed to load project Go dependencies, %v", err)
		return err
	}
	if err := updateAssetsDeps(); err != nil {
		ERROR.Printf("Failed to load project assets dependencies, %v", err)
		return err
	}

	fmt.Println()
	if err := runAndWatch(args); err != nil {
		ERROR.Printf("Failed to start watching project changes, %v", err)
		return err
	}
	return nil
}

func updateAssetsDeps() error {
	if rootConfig.Assets == nil || len(rootConfig.Assets.Dependencies) == 0 {
		return nil
	}

	fmt.Println()
	INFO.Printf("Start to loading assets dependencies...")
	cmds := []string{"install", ""}
	for _, dep := range rootConfig.Assets.Dependencies {
		cmds[len(cmds)-1] = dep
		INFO.Printf("Loading npm module: %v", dep)
		installCmd := exec.Command("npm", cmds...)
		installCmd.Stdout = os.Stdout
		installCmd.Stderr = os.Stderr
		if err := installCmd.Run(); err != nil {
			ERROR.Printf("Error when run npm install: npm %v, %v", cmds, err)
			return err
		}
	}
	SUCC.Printf("Loaded assets dependencies: \n\t%v", strings.Join(rootConfig.Assets.Dependencies, "\n\t"))
	return nil
}

func updateGolangDeps() error {
	if rootConfig.Package == nil || len(rootConfig.Package.Dependencies) == 0 {
		return nil
	}

	fmt.Println()
	INFO.Printf("Start to loading Go dependencies...")
	cmds := []string{"get", ""}
	for _, dep := range rootConfig.Package.Dependencies {
		cmds[len(cmds)-1] = dep
		INFO.Printf("Loading Go package dependency: %v", dep)
		getCmd := exec.Command("go", cmds...)
		getCmd.Stdout = os.Stdout
		getCmd.Stderr = os.Stderr
		if err := getCmd.Run(); err != nil {
			ERROR.Printf("Error when run go get: go %v, %v", cmds, err)
			return err
		}
	}
	SUCC.Printf("Loaded Go package dependencies: \n\t%v",
		strings.Join(rootConfig.Package.Dependencies, "\n\t"))
	return nil
}

var ignoreDirs = []string{".git", "node_modules"}

func runAndWatch(args []string) error {
	watcher, err := fsnotify.NewWatcher()
	if err != nil {
		return err
	}
	defer watcher.Close()

	done := make(chan bool)
	if err := addProjectDirs(".", watcher); err != nil {
		return err
	}
	go watchProjectFiles(watcher)
	INFO.Printf("Waiting for file changes...")

	app := NewAppShell(args)
	if err := app.Run(); err != nil {
		return err
	}

	<-done
	return nil
}

func isIgnoredDir(dir string) bool {
	cleanPath := strings.ToLower(path.Clean(dir))
	for _, ignore := range ignoreDirs {
		if strings.HasPrefix(cleanPath, ignore) {
			return true
		}
	}
	return false
}

func addProjectDirs(root string, watcher *fsnotify.Watcher) error {
	if err := watcher.Add(root); err != nil {
		return err
	}
	INFO.Println("Watching", root)
	return filepath.Walk(root, func(fname string, info os.FileInfo, err error) error {
		if fname == root {
			return nil
		}
		if info.IsDir() && !isIgnoredDir(fname) {
			if err := addProjectDirs(fname, watcher); err != nil {
				return err
			}
		}
		return nil
	})
}

func watchProjectFiles(watcher *fsnotify.Watcher) {
	for {
		select {
		case event := <-watcher.Events:
			if event.Name == "" || isIgnoredDir(event.Name) {
				break
			}

			if event.Op&fsnotify.Create == fsnotify.Create {
				INFO.Println("Created,", event.Name)
				if fi, err := os.Stat(event.Name); err == nil && fi.IsDir() {
					if err := watcher.Add(event.Name); err != nil {
						ERROR.Printf("Failed to add new directory into watching list[%v], %v",
							event.Name, err)
					}
				}
			} else if event.Op&fsnotify.Remove == fsnotify.Remove {
				INFO.Println("Removed,", event.Name)
				// maybe remove some dir
				if fi, err := os.Stat(event.Name); err == nil && fi.IsDir() {
					if err := watcher.Remove(event.Name); err != nil {
						ERROR.Printf("Failed to remove directory from watching list [%v], %v",
							event.Name, err)
					}
				}
				// maybe remove some source code
				// TODO
			} else if event.Op&fsnotify.Write == fsnotify.Write {
				INFO.Println(event)
			}
		case err := <-watcher.Errors:
			ERROR.Println("Error:", err)
		}
	}
}

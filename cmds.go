package main

import (
	"fmt"
	"os"
	"os/exec"
	"path"
	"path/filepath"
	"strings"
	"sync"
	"time"

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
	if err := NewProjectWatcher().runAndWatch(".", args); err != nil {
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

type ProjectWatcher struct {
	watcher    *fsnotify.Watcher
	app        *AppShell
	ignoreDirs []string
	stopChan   chan struct{}

	taskLock sync.Mutex
	tasks    []AppShellTask
}

func NewProjectWatcher() *ProjectWatcher {
	return &ProjectWatcher{
		ignoreDirs: []string{".git", "node_modules"},
		stopChan:   make(chan struct{}),
		tasks:      make([]AppShellTask, 0),
	}
}

func (pw *ProjectWatcher) runAndWatch(dir string, appArgs []string) error {
	if watcher, err := fsnotify.NewWatcher(); err != nil {
		return err
	} else {
		pw.watcher = watcher
		defer pw.watcher.Close()

		if err := pw.addDirs(dir); err != nil {
			return err
		}
		pw.app = NewAppShell(appArgs)
		go pw.watchProject()

		INFO.Printf("Waiting for file changes ...")
		if err := pw.app.Run(); err != nil {
			return err
		}

		<-pw.stopChan
		return nil
	}
}

func (pw *ProjectWatcher) isIgnoredDir(dir string) bool {
	cleanPath := strings.ToLower(path.Clean(dir))
	for _, ignore := range pw.ignoreDirs {
		if strings.HasPrefix(cleanPath, ignore) {
			return true
		}
	}
	return false
}

func (pw *ProjectWatcher) addDirs(root string) error {
	if err := pw.watcher.Add(root); err != nil {
		return err
	}
	INFO.Println("Watching", root)
	return filepath.Walk(root, func(fname string, info os.FileInfo, err error) error {
		if fname == root {
			return nil
		}
		if info.IsDir() && !pw.isIgnoredDir(fname) {
			if err := pw.addDirs(fname); err != nil {
				return err
			}
		}
		return nil
	})
}

func (pw *ProjectWatcher) addTask(taskType TaskType, module string) {

}

func (pw *ProjectWatcher) watchProject() {
	tick := time.Tick(500 * time.Millisecond)
	for {
		select {
		case event := <-pw.watcher.Events:
			if event.Name == "" || pw.isIgnoredDir(event.Name) {
				break
			}
			INFO.Println("fs event:", event)

			if event.Op&fsnotify.Create == fsnotify.Create {
				if fi, err := os.Stat(event.Name); err == nil && fi.IsDir() {
					if err := pw.watcher.Add(event.Name); err != nil {
						ERROR.Printf("Failed to add new directory into watching list[%v], %v",
							event.Name, err)
					}
				}
			} else if event.Op&fsnotify.Remove == fsnotify.Remove {
				// maybe remove some dir
				if fi, err := os.Stat(event.Name); err == nil && fi.IsDir() {
					if err := pw.watcher.Remove(event.Name); err != nil {
						ERROR.Printf("Failed to remove directory from watching list [%v], %v",
							event.Name, err)
					}
				}
				// maybe remove some source code
				// TODO
			} else if event.Op&fsnotify.Write == fsnotify.Write {

			}
		case err := <-pw.watcher.Errors:
			ERROR.Println("Error:", err)
		case <-tick:
			pw.taskLock.Lock()
			if len(pw.tasks) > 0 {

			}
			pw.taskLock.Unlock()
		}
	}
}

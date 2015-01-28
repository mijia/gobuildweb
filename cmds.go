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

	"github.com/BurntSushi/toml"
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
	rootConfig.RLock()
	defer rootConfig.RUnlock()

	if rootConfig.Assets == nil || len(rootConfig.Assets.Dependencies) == 0 {
		return nil
	}

	fmt.Println()
	INFO.Printf("Start to loading assets dependencies...")
	checkParams := []string{"list", "--depth", "0", ""}
	params := []string{"install", ""}
	deps := make([]string, len(rootConfig.Assets.Dependencies), len(rootConfig.Assets.Dependencies)+1)
	copy(deps, rootConfig.Assets.Dependencies)
	// add all dev deps for xxxify
	deps = append(deps, "browserify", "envify", "uglifyify", "reactify")
	for _, dep := range deps {
		checkParams[len(checkParams)-1] = dep
		INFO.Printf("Checking npm module: %v", dep)
		listCmd := exec.Command("npm", checkParams...)
		if err := listCmd.Run(); err == nil {
			// the module has been installed
			continue
		}

		params[len(params)-1] = dep
		INFO.Printf("Loading npm module: %v", dep)
		installCmd := exec.Command("npm", params...)
		installCmd.Stdout = os.Stdout
		installCmd.Stderr = os.Stderr
		if err := installCmd.Run(); err != nil {
			ERROR.Printf("Error when run npm install: npm %v, %v", params, err)
			return err
		}
	}
	SUCC.Printf("Loaded assets dependencies: \n\t%v", strings.Join(deps, "\n\t"))
	return nil
}

func updateGolangDeps() error {
	rootConfig.RLock()
	defer rootConfig.RUnlock()

	if rootConfig.Package == nil || len(rootConfig.Package.Dependencies) == 0 {
		return nil
	}

	fmt.Println()
	INFO.Printf("Start to loading Go dependencies...")
	params := []string{"get", ""}
	for _, dep := range rootConfig.Package.Dependencies {
		params[len(params)-1] = dep
		INFO.Printf("Loading Go package dependency: %v", dep)
		getCmd := exec.Command("go", params...)
		getCmd.Stdout = os.Stdout
		getCmd.Stderr = os.Stderr
		if err := getCmd.Run(); err != nil {
			ERROR.Printf("Error when run go get: go %v, %v", params, err)
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
		ignoreDirs: []string{".git", "node_modules", "public"},
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
	return filepath.Walk(root, func(fname string, info os.FileInfo, err error) error {
		if info.IsDir() && !pw.isIgnoredDir(fname) {
			if err := pw.watcher.Add(fname); err != nil {
				return err
			}
			INFO.Println("Watching", fname)
		}
		return nil
	})
}

func (pw *ProjectWatcher) addTask(taskType TaskType, module string) {
	pw.taskLock.Lock()
	defer pw.taskLock.Unlock()

	added := false
	newTask := AppShellTask{taskType, module}
	for i, task := range pw.tasks {
		if task.taskType == taskType && task.module == module {
			return
		}
		if task.taskType > taskType {
			pw.tasks = append(pw.tasks[:i], append([]AppShellTask{newTask}, pw.tasks[i:]...)...)
			added = true
			break
		}
	}
	if !added {
		pw.tasks = append(pw.tasks, newTask)
	}
}

func (pw *ProjectWatcher) hasGoTests(module string) bool {
	has := false
	ignoreTests := make(map[string]struct{})
	rootConfig.RLock()
	for _, t := range rootConfig.Package.OmitTests {
		ignoreTests[t] = struct{}{}
	}
	rootConfig.RUnlock()
	err := filepath.Walk(module, func(fname string, info os.FileInfo, err error) error {
		if _, ok := ignoreTests[fname]; !ok && !info.IsDir() {
			if strings.HasSuffix(fname, "_test.go") {
				has = true
			}
		}
		return nil
	})
	return err == nil && has
}

func (pw *ProjectWatcher) updateConfig() {
	INFO.Println("Reloading the project.toml file ...")
	var newConfig ProjectConfig
	if _, err := toml.DecodeFile("project.toml", &newConfig); err != nil {
		ERROR.Printf("We found the project.toml has changed, but it contains some error, will omit it.")
		ERROR.Printf("TOML Error: %v", err)
		fmt.Println()
		INFO.Println("Waiting for the file changes ...")
	} else {
		SUCC.Printf("Loaded the new project.toml, will update all the dependencies ...")
		rootConfig.Lock()
		rootConfig.Package = newConfig.Package
		rootConfig.Assets = newConfig.Assets
		rootConfig.Distribution = newConfig.Distribution
		rootConfig.Unlock()
		if err := updateGolangDeps(); err != nil {
			ERROR.Printf("Failed to load project Go dependencies, %v", err)
			return
		}
		if err := updateAssetsDeps(); err != nil {
			ERROR.Printf("Failed to load project assets dependencies, %v", err)
			return
		}
		pw.addTask(kTaskBuildImages, "")
		pw.addTask(kTaskBuildStyles, "")
		pw.addTask(kTaskBuildJavaScripts, "")
		pw.addTask(kTaskBuildBinary, "")
	}
}

func (pw *ProjectWatcher) watchProject() {
	tick := time.Tick(800 * time.Millisecond)
	for {
		select {
		case event := <-pw.watcher.Events:
			if event.Name == "" || pw.isIgnoredDir(event.Name) {
				break
			}

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
					// if the dir is under assets, we need to rebuild all the assets or sprites
					// else we take it as a go code directory
					// TODO
				}
				// maybe remove some source code
				// TODO
			} else if event.Op&fsnotify.Write == fsnotify.Write {
				if event.Name == "project.toml" {
					pw.updateConfig()
				}
				if strings.HasSuffix(event.Name, ".go") {
					goModule := path.Dir(event.Name)
					if pw.hasGoTests(goModule) {
						pw.addTask(kTaskBinaryTest, goModule)
					}
					pw.addTask(kTaskBuildBinary, goModule)
				}
				// js, css, file changes
				// sprite images updated
			}
		case err := <-pw.watcher.Errors:
			ERROR.Println("Error:", err)
		case <-tick:
			pw.taskLock.Lock()
			if len(pw.tasks) > 0 {
				pw.app.executeTask(pw.tasks...)
				pw.tasks = make([]AppShellTask, 0)
			}
			pw.taskLock.Unlock()
		}
	}
}

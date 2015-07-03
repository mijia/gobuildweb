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
	"github.com/mijia/gobuildweb/loggers"
	"gopkg.in/fsnotify.v1"
)

type Command func(args []string) error

func commandDist(args []string) error {
	if err := updateGolangDeps(); err != nil {
		loggers.Error("Failed to load project #Golang dependencies, %v", err)
		return err
	}
	if err := updateAssetsDeps(); err != nil {
		loggers.Error("Failed to load project assets dependencies, %v", err)
		return err
	}

	return NewAppShell(args).Dist()
}

func commandRun(args []string) error {
	if err := updateGolangDeps(); err != nil {
		loggers.Error("Failed to load project Go dependencies, %v", err)
		return err
	}
	if err := updateAssetsDeps(); err != nil {
		loggers.Error("Failed to load project assets dependencies, %v", err)
		return err
	}

	fmt.Println()
	if err := NewProjectWatcher().runAndWatch(".", args); err != nil {
		loggers.Error("Failed to start watching project changes, %v", err)
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
	loggers.Info("Start to loading assets dependencies...")
	checkParams := []string{"list", "--depth", "0", ""}
	params := []string{"install", ""}
	deps := make([]string, len(rootConfig.Assets.Dependencies), len(rootConfig.Assets.Dependencies)+1)
	copy(deps, rootConfig.Assets.Dependencies)
	// add all dev deps for xxxify
	deps = append(deps, "browserify", "coffeeify", "envify", "uglifyify", "babelify", "nib", "stylus")
	for _, dep := range deps {
		checkParams[len(checkParams)-1] = dep
		listCmd := exec.Command("npm", checkParams...)
		listCmd.Env = mergeEnv(nil)
		if err := listCmd.Run(); err == nil {
			// the module has been installed
			loggers.Info("Checked npm module: %v", dep)
			continue
		}

		params[len(params)-1] = dep
		loggers.Info("Loading npm module: %v", dep)
		installCmd := exec.Command("npm", params...)
		installCmd.Stdout = os.Stdout
		installCmd.Stderr = os.Stderr
		installCmd.Env = mergeEnv(nil)
		if err := installCmd.Run(); err != nil {
			loggers.Error("Error when run npm install: npm %v, %v", params, err)
			return err
		}
	}
	loggers.Succ("Loaded assets dependencies: \n\t%v", strings.Join(deps, "\n\t"))
	return nil
}

func updateGolangDeps() error {
	rootConfig.RLock()
	defer rootConfig.RUnlock()

	if rootConfig.Package == nil || len(rootConfig.Package.Dependencies) == 0 {
		return nil
	}

	fmt.Println()
	loggers.Info("Start to loading Go dependencies...")
	params := []string{"get", ""}
	for _, dep := range rootConfig.Package.Dependencies {
		params[len(params)-1] = dep
		loggers.Info("Loading Go package dependency: %v", dep)
		getCmd := exec.Command("go", params...)
		getCmd.Stdout = os.Stdout
		getCmd.Stderr = os.Stderr
		getCmd.Env = mergeEnv(nil)
		if err := getCmd.Run(); err != nil {
			loggers.Error("Error when run go get: go %v, %v", params, err)
			return err
		}
	}
	loggers.Succ("Loaded Go package dependencies: \n\t%v",
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
		pw.app = NewAppShell(appArgs)
		if err := pw.app.Run(); err != nil {
			return err
		}

		pw.watcher = watcher
		defer pw.watcher.Close()
		if err := pw.addDirs(dir); err != nil {
			return err
		}
		go pw.watchProject()
		loggers.Info("Waiting for file changes ...")

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
		if err == nil && info.IsDir() && !pw.isIgnoredDir(fname) {
			if err := pw.watcher.Add(fname); err != nil {
				return err
			}
			loggers.Debug("Watching %s", fname)
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
		if _, ok := ignoreTests[fname]; !ok {
			if !info.IsDir() {
				if strings.HasSuffix(fname, "_test.go") {
					has = true
				}
			} else if fname != module {
				return filepath.SkipDir
			}
		}
		return nil
	})
	return err == nil && has
}

func (pw *ProjectWatcher) updateConfig() {
	loggers.Info("Reloading the project.toml file ...")
	var newConfig ProjectConfig
	if _, err := toml.DecodeFile("project.toml", &newConfig); err != nil {
		loggers.Error("We found the project.toml has changed, but it contains some error, will omit it.")
		loggers.Error("TOML Error: %v", err)
		fmt.Println()
		loggers.Info("Waiting for the file changes ...")
	} else {
		loggers.Succ("Loaded the new project.toml, will update all the dependencies ...")
		rootConfig.Lock()
		rootConfig.Package = newConfig.Package
		rootConfig.Assets = newConfig.Assets
		rootConfig.Distribution = newConfig.Distribution
		rootConfig.Unlock()
		if err := updateGolangDeps(); err != nil {
			loggers.Error("Failed to load project Go dependencies, %v", err)
			return
		}
		if err := updateAssetsDeps(); err != nil {
			loggers.Error("Failed to load project assets dependencies, %v", err)
			return
		}
		pw.addTask(kTaskBuildImages, "")
		pw.addTask(kTaskBuildStyles, "")
		pw.addTask(kTaskBuildJavaScripts, "")
		pw.addTask(kTaskGenAssetsMapping, "")
		pw.addTask(kTaskBuildBinary, "")
		pw.addTask(kTaskBinaryRestart, "")
	}
}

func (pw *ProjectWatcher) goModuleName(dir string) (string, error) {
	if dir == "." {
		return dir, nil
	}
	if absPath, err := filepath.Abs(dir); err != nil {
		return "", err
	} else {
		goPath := os.Getenv("GOPATH")
		if !strings.HasPrefix(absPath, goPath) {
			return "", fmt.Errorf("Go module not in GOPATH[%s]", goPath)
		}
		return absPath[len(goPath)+len("/src/"):], nil
	}
}

func (pw *ProjectWatcher) maybeGoCodeChanged(fname string) {
	if strings.HasSuffix(fname, ".go") {
		goModule := path.Dir(fname)
		if pw.hasGoTests(goModule) {
			if moduleName, err := pw.goModuleName(goModule); err == nil {
				pw.addTask(kTaskBinaryTest, moduleName)
			} else {
				loggers.Error("Cannot get go module path name, %v", err)
			}
		}
		pw.addTask(kTaskBuildBinary, goModule)
		pw.addTask(kTaskBinaryRestart, "")
	}
}

func (pw *ProjectWatcher) maybeAssetsChanged(fname string) {
	if !strings.HasPrefix(fname, "assets/") {
		return
	}
	categories := []string{"assets/images/", "assets/stylesheets/", "assets/javascripts/"}
	taskTypes := []TaskType{kTaskBuildImages, kTaskBuildStyles, kTaskBuildJavaScripts}
	for i, category := range categories {
		if strings.HasPrefix(fname, category) {
			name := fname[len(category):]
			if index := strings.Index(name, "/"); index != -1 {
				name = name[:index]
			} else {
				name = name[:len(name)-len(filepath.Ext(name))]
			}
			if _, ok := rootConfig.getAssetEntry(name); ok {
				pw.addTask(taskTypes[i], name)
			} else {
				// we naively think this as a global change
				pw.addTask(taskTypes[i], "")
			}
			pw.addTask(kTaskGenAssetsMapping, "")
		}
	}
}

func (pw *ProjectWatcher) watchProject() {
	tick := time.Tick(800 * time.Millisecond)
	for {
		select {
		case event := <-pw.watcher.Events:
			if event.Name == "" ||
				pw.isIgnoredDir(event.Name) ||
				strings.HasSuffix(event.Name, ".swp") ||
				strings.HasSuffix(event.Name, ".DS_Store") {
				break
			}
			loggers.Debug("fsevents: %v", event)
			if event.Op&fsnotify.Create == fsnotify.Create || event.Op&fsnotify.Write == fsnotify.Write {
				if fi, err := os.Stat(event.Name); err == nil {
					if fi.IsDir() {
						if err := pw.watcher.Add(event.Name); err != nil {
							loggers.Error("Failed to add new directory into watching list[%v], %v",
								event.Name, err)
						} else {
							loggers.Debug("Watching %s", event.Name)
						}
					} else {
						if event.Name == "project.toml" {
							pw.updateConfig()
						}
						pw.maybeGoCodeChanged(event.Name)
						pw.maybeAssetsChanged(event.Name)
					}
				}
			} else if event.Op&fsnotify.Remove == fsnotify.Remove || event.Op&fsnotify.Rename == fsnotify.Rename {
				// maybe remove some dir
				if fi, err := os.Stat(event.Name); err == nil {
					if fi.IsDir() {
						if err := pw.watcher.Remove(event.Name); err != nil {
							loggers.Error("Failed to remove directory from watching list [%v], %v",
								event.Name, err)
						}
						// if the dir is under assets, we need to rebuild all the assets or sprites
						// else we take it as a go code directory
						// TODO
					} else {
						if event.Name == "project.toml" {
							panic("Please don't hurt the project.toml")
						}
						// maybe remove some source code
						// TODO
					}
				}
			}
		case err := <-pw.watcher.Errors:
			loggers.Error("Error: %v", err)
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

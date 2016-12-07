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
	"runtime"
	"gopkg.in/bufio.v1"
	"github.com/mijia/gobuildweb/assets"
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

func commandWatch(args []string) error {
	fmt.Println()
	if err := NewProjectWatcher().WatchOnly(".", args); err != nil {
		loggers.Error("Failed to start watching project changes, %v", err)
		return err
	}
	return nil
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

	nodeModulesDir := "./node_modules"
	if _, err := os.Stat(nodeModulesDir); err != nil {
		err = os.Mkdir(nodeModulesDir, 0755)
		if err != nil {
			loggers.Error("Cannot create "+nodeModulesDir+": %v", err)
			return err
		}
	}
	fmt.Println()
	loggers.Info("Start to loading assets dependencies...")
	checkParams := []string{"list", "--depth", "0"}
	params := []string{"install", ""}
	deps := make([]string, len(rootConfig.Assets.Dependencies), len(rootConfig.Assets.Dependencies)+1)
	copy(deps, rootConfig.Assets.Dependencies)
	deps = append(deps, "browserify", "coffeeify", "envify", "uglifyify", "babelify", "babel-preset-es2015", "babel-preset-react", "nib", "stylus")
	notInstalledDeps := make([]string, 0)
	listCmd := exec.Command("npm", checkParams...)
	listCmd.Env = mergeEnv(nil)
	npmPackageNames := ""
	if outputs, err := listCmd.CombinedOutput(); err != nil {
		// the module has been installed
		loggers.Warn("npm check error: %v", err)
	} else {
		npmPackageNames = string(outputs)
	}

	for _, dep := range deps {

		//list format : "├── babel-preset-es2015@6.6.0"
		depName := dep;
		if !strings.HasPrefix(dep, "git+") {
			dep = "── " + dep
			if !strings.Contains(dep, "@") {
				dep += "@"
			}
		}
		if strings.Contains(npmPackageNames, dep) {
			loggers.Info("npm module %s is found", dep)
		} else {
			notInstalledDeps = append(notInstalledDeps, depName)
		}
	}

	for _, dep := range notInstalledDeps {
		params[len(params)-1] = dep
		loggers.Info("Loading npm module: %v", dep)
		installCmd := exec.Command("npm", params...)
		installCmd.Stdout = os.Stdout
		installCmd.Stderr = os.Stderr
		installCmd.Env = mergeEnv(nil)
		if err := installCmd.Run(); err != nil {
			loggers.Warn("Error when run npm install: npm %v, %v", params, err)
			//return err
		}
	}
	loggers.Succ("Loaded assets dependencies: \n\t%v", strings.Join(deps, "\n\t"))
	return nil
}
//是否已经下载了go运行所需要的包,是否可以编译成功
func hasGetColangDeps() bool {
	cmd := exec.Command("go", "build")
	loggers.Info("Started to go build...")
	err := cmd.Start()
	if err != nil {
		loggers.Error("Failed to go build... %+v", err)
		return false
	}
	done := make(chan error, 1)
	go func() {
		done <- cmd.Wait()
	}()
	err = <-done
	if err != nil {
		loggers.Error("Failed to go build...%+v", err)
	} else {
		loggers.Info("Successed to go build...")
		cmd := exec.Command("rm", "zhiwang_web")
		cmd.Run()
		return true
	}

	return false
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

func (pw *ProjectWatcher) WatchOnly(dir string, appArgs []string) error {
	if watcher, err := fsnotify.NewWatcher(); err != nil {
		return err
	} else {
		pw.app = NewAppShell(appArgs)
		pw.app.isProduction = false
		go pw.app.startRunner()
		goOs, goArch := runtime.GOOS, runtime.GOARCH
		pw.app.binName = pw.app.binaryName(rootConfig.Package.Name, rootConfig.Package.Version, goOs, goArch)
		if _, err := os.Stat(pw.app.binName); err != nil {
			loggers.Warn(pw.app.binName + " does not exist, binaryBuild start!")
			pw.app.executeTask(
				AppShellTask{kTaskBuildBinary, ""},
				AppShellTask{kTaskBinaryRestart, ""},
			)
		} else {
			pw.app.executeTask(
				AppShellTask{kTaskBinaryRestart, ""},
			)
		}
		pw.watcher = watcher
		defer func(){
			pw.watcher.Close()
		}()
		if err := pw.addDirs(dir); err != nil {
			return err
		}

		go pw.watchProject()
		loggers.Info("Waiting for file changes ...")

		<-pw.stopChan
		return nil
	}
}
func (pw *ProjectWatcher) runAndWatch(dir string, appArgs []string) error {
	if watcher, err := fsnotify.NewWatcher(); err != nil {
		return err
	} else {
		pw.watcher = watcher
		pw.app = NewAppShell(appArgs)
		if err := pw.app.Run(); err != nil {
			return err
		}
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
			if info != nil && !info.IsDir() {
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

func stringListIsEqual(s1, s2 []string) bool {
	if len(s1) != len(s2) {
		return false
	}
	sMap := map[string]bool{}
	for _, s:= range s1 {
		sMap[strings.TrimSpace(s)] = true
	}
	for _, s := range s2 {
		if _, ok := sMap[strings.TrimSpace(s)]; !ok {
			return false
		}
	}
	return true
}


//return diff entry name
func assetConfigDiff(oldAssets *assets.Config, newAssets *assets.Config) (entryList []string, needBuildAll bool) {
	if newAssets == nil {
		return nil, false
	}
	if oldAssets == nil {
		return nil, true
	}
	if oldAssets == nil || oldAssets.UrlPrefix != newAssets.UrlPrefix ||
	oldAssets.AssetsMappingPkg != newAssets.AssetsMappingPkg ||
	oldAssets.AssetsMappingPkgRelative!= newAssets.AssetsMappingPkgRelative ||
	oldAssets.AssetsMappingJson != newAssets.AssetsMappingJson ||
	!stringListIsEqual(oldAssets.ImageExts, newAssets.ImageExts) ||
	!stringListIsEqual(oldAssets.Dependencies, newAssets.Dependencies){
		return nil, true
	}
	diffNames := entryListDiff(oldAssets.VendorSets, newAssets.VendorSets)
	diffNames = append(diffNames, entryListDiff(oldAssets.Entries, newAssets.Entries)...)
	return diffNames, false
}

func entryIsEqual(e1 *assets.Entry, e2 *assets.Entry) bool {
	if e1 == nil && e2 == nil {
		return true
	}
	if e1 == nil || e2 == nil {
		return false
	}
	if e1.Name != e2.Name || !stringListIsEqual(e1.Requires, e2.Requires) ||
		!stringListIsEqual(e1.Externals, e2.Externals) ||
		!stringListIsEqual(e1.Dependencies, e2.Dependencies) ||
		!stringListIsEqual(e1.BundleOpts, e2.BundleOpts) {
		return false
	}
	return true
}
func entryListDiff(oldList []*assets.Entry, newList []*assets.Entry) []string {
	entryMap := make(map[string]*assets.Entry, len(oldList))
	for _, e := range oldList {
		entryMap[e.Name] = e
	}

	diffNames := []string{}
	for _, e := range newList {
		if oldEntry, ok := entryMap[e.Name]; ok {
			if !entryIsEqual(e, oldEntry) {
				diffNames = append(diffNames, e.Name)
			}
		} else {
			diffNames = append(diffNames, e.Name)
		}
	}
	return diffNames
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

		needUpdateGoDeps := false
		if rootConfig.Package != nil &&  newConfig.Package != nil {
			needUpdateGoDeps = !stringListIsEqual(newConfig.Package.Dependencies, rootConfig.Package.Dependencies)
		}

		needUpdateAssetDeps := false
		if rootConfig.Assets!= nil &&  newConfig.Assets!= nil {
			needUpdateAssetDeps = !stringListIsEqual(newConfig.Assets.Dependencies, rootConfig.Assets.Dependencies)
		}

		p := rootConfig.Package
		pp := newConfig.Package
		needBuildBinary := !(p != nil && pp != nil && p.Name == pp.Name &&
							p.Version == pp.Version &&
							p.Builder == pp.Builder &&
							p.IsGraceful == pp.IsGraceful &&
							stringListIsEqual(p.BuildOpts, pp.BuildOpts) &&
							stringListIsEqual(p.OmitTests, pp.OmitTests))
		diffEntryNames, needBuildAllAssets := assetConfigDiff(rootConfig.Assets, newConfig.Assets)

		rootConfig.Package = newConfig.Package
		rootConfig.Assets = newConfig.Assets
		rootConfig.Distribution = newConfig.Distribution
		rootConfig.Unlock()

		if needUpdateGoDeps {
			if err := updateGolangDeps(); err != nil {
				loggers.Error("Failed to load project Go dependencies, %v", err)
				return
			}
		}

		if needUpdateAssetDeps {
			if err := updateAssetsDeps(); err != nil {
				loggers.Error("Failed to load project assets dependencies, %v", err)
				return
			}
		}
		if needBuildAllAssets {
			pw.addTask(kTaskBuildImages, "")
			pw.addTask(kTaskGenAssetsMapping, "")
			pw.addTask(kTaskBuildStyles, "")
			pw.addTask(kTaskBuildJavaScripts, "")
			pw.addTask(kTaskGenAssetsMapping, "")
		} else {
			for _, name := range diffEntryNames {
				pw.addTask(kTaskBuildImages, name)
				pw.addTask(kTaskGenAssetsMapping, "")
				pw.addTask(kTaskBuildStyles, name)
				pw.addTask(kTaskBuildJavaScripts, name)
				pw.addTask(kTaskGenAssetsMapping, "")
			}
		}
		if needBuildBinary{
			pw.addTask(kTaskBuildBinary, "")
			pw.addTask(kTaskBinaryRestart, "")
		}
	}
	loggers.Info("Reloading the project.toml file Finished!")
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
		loggers.Info(fname + " has been changed, buildBinary starts!")
		pw.app.stopBuildBinary()
		pw.addTask(kTaskBuildBinary, goModule)
		pw.addTask(kTaskBinaryRestart, "")
	}
}

type DepsGetFunc func(string, string) []string
func imageDepsGet(name string, path string) []string {
	return []string {""}
}
func styleDepsGet(name string, path string) []string {
	return []string {""}
}
func javascriptDepsGet(name string, path string) []string {
	return []string {""}
}

func (pw *ProjectWatcher) maybeAssetsChanged(fname string) {
	if !strings.HasPrefix(fname, "assets/") {
		return
	}
	categories := []string{"assets/images/", "assets/stylesheets/", "assets/javascripts/"}
	taskTypes := []TaskType{kTaskBuildImages, kTaskBuildStyles, kTaskBuildJavaScripts}
	depsGet := []DepsGetFunc {imageDepsGet, styleDepsGet, javascriptDepsGet}
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
				depsList := depsGet[i](name, categories[i])
				for _, dep := range depsList {
					pw.addTask(taskTypes[i], dep)
				}
			}
			loggers.Info(fname + " has been changed!")
			pw.addTask(kTaskGenAssetsMapping, "")
		}
	}
}

func (pw *ProjectWatcher) watchProject() {
	/*
	defer func(){
		loggers.Info("Defer here, kill App")
		pw.app.kill()
		loggers.Info("Leaving gobuildweb, bye!")
		pw.watcher.Close()
		os.Exit(0)
	}()
	*/
	go func(){
		reader := bufio.NewReader(os.Stdin)
		for {
			fmt.Print( "build-cmd>")
			str, err := reader.ReadString('\n')
			cmd := ""
			args := []string{}
			if(err == nil) {
				strList := strings.Split(strings.TrimSpace(str), " ")
				if len(strList) > 0 {
					cmd = strings.TrimSpace(strList[0])
				}
				args = strList[1:]
			} else {
				cmd = "quit"
			}
			if cmd == "b" || cmd=="bin" || cmd=="binary" {
				fmt.Println( "start to build binary, wait for a while!\n")
				pw.app.executeTask(
					AppShellTask{kTaskBuildBinary, ""},
					AppShellTask{kTaskBinaryRestart, ""},
				)
			} else if cmd=="s" || cmd=="style" || cmd=="styles" {
				if len(args)>0 {
					for _, arg := range args {
						pw.app.executeTask(
							AppShellTask{kTaskBuildStyles, arg},
						)
					}
				} else {
					pw.app.executeTask(AppShellTask{kTaskBuildStyles, ""})
				}
				pw.app.executeTask(AppShellTask{kTaskGenAssetsMapping, ""})
			} else if cmd=="i" || cmd=="image" || cmd=="images" {
				if len(args)>0 {
					for _, arg := range args {
						pw.app.executeTask(
							AppShellTask{kTaskBuildImages, arg},
						)
					}
				} else {
					pw.app.executeTask(AppShellTask{kTaskBuildImages, ""})
				}
				pw.app.executeTask(AppShellTask{kTaskGenAssetsMapping, ""})
			} else if cmd=="j" || cmd=="js" || cmd=="javascript"{
				if len(args)>0 {
					for _, arg := range args {
						pw.app.executeTask(
							AppShellTask{kTaskBuildJavaScripts, arg},
						)
					}
				} else {
					pw.app.executeTask(AppShellTask{kTaskBuildJavaScripts, ""})
				}
				pw.app.executeTask(AppShellTask{kTaskGenAssetsMapping, ""})
			} else if cmd=="q" || cmd=="quit" || cmd=="exit" {
				fmt.Println( "quit gobuildweb!\n")
				pw.app.kill()
				fmt.Println( "Bye!\n")
				os.Exit(0)
			} else {
				fmt.Println( "b,bin,binary : rebuild binarys; \n"+
				"s,style,styles [entry1 entry2 ...]: rebuild styles; \n"+
				"i,image,images [entry1 entry2 ...]: rebuild images; \n"+
				"j,js,javascript [entry1 entry2 ...] : rebuild javascript; \n"+
				"q,quit,exit: quit gobuildweb\n" )
			}
		}
	}()
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
						/*
						if event.Name == "project.toml" {
							panic("Please don't hurt the project.toml")
						}
						*/
						if fi.Name() == "project.toml" {
							//pw.updateConfig()
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

package main

import (
	"os"
	"path"
	"strings"

	"gopkg.in/fsnotify.v1"
)

var ignoreDirs = []string{"node_modules"}

type Project struct {
	args []string
}

func (p *Project) WatchRun() error {
	watcher, err := fsnotify.NewWatcher()
	if err != nil {
		return err
	}
	defer watcher.Close()

	done := make(chan bool)
	if err := p.addProjectDirs(watcher); err != nil {
		return err
	}
	go p.watchProjectFiles(watcher)
	INFO.Printf("Waiting for file changes...")
	<-done
	return nil
}

func (p *Project) isIgnoredDir(dir string) bool {
	cleanPath := strings.ToLower(path.Clean(dir))
	for _, ignore := range ignoreDirs {
		if strings.HasPrefix(cleanPath, ignore) {
			return true
		}
	}
	return false
}

func (p *Project) addProjectDirs(watcher *fsnotify.Watcher) error {
	if err := watcher.Add("."); err != nil {
		return err
	}

	return nil
}

func (p *Project) watchProjectFiles(watcher *fsnotify.Watcher) {
	for {
		select {
		case event := <-watcher.Events:
			if event.Name == "" || p.isIgnoredDir(event.Name) {
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

func NewProject(args []string) *Project {
	return &Project{
		args: args,
	}
}

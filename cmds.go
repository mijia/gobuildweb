package main

import (
	"bytes"
	"fmt"
	"os/exec"
	"strings"
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

	return nil
}

func commandRun(args []string) error {
	if err := updateGolangDeps(); err != nil {
		ERROR.Printf("Failed to load project #Golang dependencies, %v", err)
		return err
	}
	if err := updateAssetsDeps(); err != nil {
		ERROR.Printf("Failed to load project assets dependencies, %v", err)
		return err
	}

	fmt.Println()
	project := NewProject(args)
	if err := project.WatchRun(); err != nil {
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
	INFO.Printf("Start to loading assets #NPM dependencies...")
	cmds := []string{"install", ""}
	for _, dep := range rootConfig.Assets.Dependencies {
		cmds[len(cmds)-1] = dep
		INFO.Printf("Loading NPM module: %v", dep)
		installCmd := exec.Command("npm", cmds...)
		var errorPipe bytes.Buffer
		installCmd.Stderr = &errorPipe
		if err := installCmd.Run(); err != nil {
			ERROR.Printf("Error when run npm install: npm %v\n%v", cmds, errorPipe.String())
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
	INFO.Printf("Start to loading #Golang dependencies...")
	cmds := []string{"get", ""}
	for _, dep := range rootConfig.Package.Dependencies {
		cmds[len(cmds)-1] = dep
		INFO.Printf("Loading Go lib dependency: %v", dep)
		getCmd := exec.Command("go", cmds...)
		var errorPipe bytes.Buffer
		getCmd.Stderr = &errorPipe
		if err := getCmd.Run(); err != nil {
			ERROR.Printf("Error when run go get: go %v\n%v", cmds, errorPipe.String())
			return err
		}
	}
	SUCC.Printf("Loaded Go lib dependencies: \n\t%v", strings.Join(rootConfig.Package.Dependencies, "\n\t"))
	return nil
}

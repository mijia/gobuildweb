package main

import (
	"bytes"
	"fmt"
	"os/exec"
	"strings"
)

type Command func(args []string) error

func commandDist(args []string) error {
	return nil
}

func commandUpdate(args []string) error {
	fmt.Println()
	INFO.Printf("Start to loading dependencies...")
	cmds := make([]string, 0, 2+len(args))
	cmds = append(cmds, "get")
	cmds = append(cmds, args...)
	cmds = append(cmds, "")
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

func commandRun(args []string) error {
	if err := commandUpdate(args); err != nil {
		ERROR.Printf("Failed to load project dependencies, %v", err)
		return err
	}
	fmt.Println()
	INFO.Printf("Waiting for the file changes...")

	return nil
}

var _ = fmt.Println

package main

import (
	"bytes"
	"fmt"
	"os/exec"
)

type Command func(args []string) error

func commandTest(args []string) error {
	return nil
}

func commandDist(args []string) error {
	return nil
}

func commandUpdate(args []string) error {
	INFO.Printf("Start to loading dependencies...")
	cmds := make([]string, 0, 4)
	cmds = append(cmds, "get")
	for _, arg := range args {
		cmds = append(cmds, arg)
	}
	cmds = append(cmds, "")
	for _, dep := range rootConfig.Package.Dependencies {
		cmds[len(cmds)-1] = dep
		getCmd := exec.Command("go", cmds...)
		var (
			outputPipe bytes.Buffer
			errorPipe  bytes.Buffer
		)
		getCmd.Stdout = &outputPipe
		getCmd.Stderr = &errorPipe
		if err := getCmd.Run(); err != nil {
			ERROR.Printf("Error when run go get: go %v\n%v", cmds, errorPipe.String())
			return err
		} else {
			SUCC.Printf("Loaded dependency: %v %v", dep, outputPipe.String())
		}
	}
	return nil
}

func commandRun(args []string) error {
	if err := commandUpdate(args); err != nil {
		ERROR.Printf("Failed to load project dependencies, %v", err)
		return err
	}

	return nil
}

var _ = fmt.Println

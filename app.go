package main

import "fmt"

type AppShell struct {
	args []string
}

func (app *AppShell) Run() error {
	return nil
}

func (app *AppShell) Dist() error {
	fmt.Println()
	INFO.Printf("Creating distribution package for %v-%v",
		rootConfig.Package.Name, rootConfig.Package.Version)
	return nil
}

func (app *AppShell) buildAssets() error {
	return nil
}

func (app *AppShell) buildBinary() error {
	return nil
}

func NewAppShell(args []string) *AppShell {
	return &AppShell{
		args: args,
	}
}

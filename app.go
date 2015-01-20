package main

import "fmt"

type WebApp struct {
	args []string
}

func (app *WebApp) Run() error {
	return nil
}

func (app *WebApp) Dist() error {
	fmt.Println()
	INFO.Printf("Creating distribution package for %v-%v",
		rootConfig.Package.Name, rootConfig.Package.Version)
	return nil
}

func (app *WebApp) buildAssets() error {
	return nil
}

func (app *WebApp) buildBinary() error {
	return nil
}

func NewWebApp(args []string) *WebApp {
	return &WebApp{
		args: args,
	}
}

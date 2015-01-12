package main

import (
	"fmt"
)

type Command func(args []string) error

func dummyCommand(args []string) error {
	fmt.Println(args)
	return nil
}

package main

import (
	"fmt"
	"testing"
)

func TestSimple(t *testing.T) {
	if false {
		t.Errorf("This is a error")
	}
	fmt.Println("Hello")
}

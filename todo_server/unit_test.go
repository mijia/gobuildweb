package main

import "testing"

func TestSimple(t *testing.T) {
	if false {
		t.Errorf("This is a error")
	}
}

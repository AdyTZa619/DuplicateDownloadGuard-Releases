package main

import (
	"reflect"
	"testing"
)

func TestMegaPublicLoginArgsV856UsesResume(t *testing.T) {
	got := megaPublicLoginArgsV856("  https://mega.nz/folder/ABC#KEY  ")
	want := []string{"login", "https://mega.nz/folder/ABC#KEY", "--resume"}
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("login args = %#v, want %#v", got, want)
	}
}

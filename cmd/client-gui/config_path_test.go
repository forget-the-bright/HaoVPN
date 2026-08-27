package main

import (
	"path/filepath"
	"testing"
)

func TestResolveClientConfigPathNonEmpty(t *testing.T) {
	p := resolveClientConfigPath()
	if p == "" {
		t.Fatal("empty path")
	}
	if filepath.Base(p) != "client.yaml" {
		t.Fatalf("base=%s", filepath.Base(p))
	}
}

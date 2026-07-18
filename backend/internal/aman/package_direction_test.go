package aman

import (
	"os/exec"
	"strings"
	"testing"
)

func TestCorePackageDependsOnlyOnStandardLibrary(t *testing.T) {
	command := exec.Command("go", "list", "-f", "{{join .Imports \"\\n\"}}", ".")
	output, err := command.Output()
	if err != nil {
		t.Fatalf("list direct package imports: %v", err)
	}
	for _, importPath := range strings.Fields(string(output)) {
		if strings.Contains(importPath, "/") || strings.Contains(importPath, ".") {
			t.Errorf("core AMAN package must not import %q; adapters and owning packages depend inward", importPath)
		}
	}
}

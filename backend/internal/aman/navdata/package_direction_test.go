package navdata

import (
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"testing"
)

func TestRuntimePackagesDoNotImportNavigationSources(t *testing.T) {
	_, file, _, ok := runtime.Caller(0)
	if !ok {
		t.Fatal("resolve package path")
	}
	internal := filepath.Dir(filepath.Dir(filepath.Dir(file)))
	for _, directory := range []string{"aman", "frontend", "repository", "predictor", "sequence"} {
		path := filepath.Join(internal, directory)
		entries, err := os.ReadDir(path)
		if os.IsNotExist(err) {
			continue
		}
		if err != nil {
			t.Fatalf("read %s: %v", path, err)
		}
		for _, entry := range entries {
			if entry.IsDir() || !strings.HasSuffix(entry.Name(), ".go") || strings.HasSuffix(entry.Name(), "_test.go") {
				continue
			}
			contents, err := os.ReadFile(filepath.Join(path, entry.Name()))
			if err != nil {
				t.Fatalf("read %s: %v", entry.Name(), err)
			}
			if strings.Contains(string(contents), "FlightStrips/internal/aman/navdata/fixture") || strings.Contains(string(contents), "FlightStrips/internal/aman/navdata/airacnet") {
				t.Errorf("%s must use cache-only navdata.GeometryReader, not a source adapter", filepath.Join(directory, entry.Name()))
			}
		}
	}
}

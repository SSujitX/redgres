package web

import (
	"io/fs"
	"testing"
)

func TestAssetsAvailableWithoutFrontendBuild(t *testing.T) {
	assets, err := Assets()
	if err != nil {
		t.Fatalf("Assets: %v", err)
	}
	if _, err := fs.Stat(assets, "."); err != nil {
		t.Fatalf("embed root: %v", err)
	}
	if Exists("definitely-not-present") {
		t.Fatal("missing file reported as present")
	}

	_, openErr := assets.Open("index.html")
	present := Exists("index.html")
	if openErr == nil && !present {
		t.Fatal("index.html opened but Exists reported false")
	}
	if openErr != nil && present {
		t.Fatal("Exists reported index.html after Open failed")
	}
	if openErr != nil {
		// Clean-tree / CI placeholder: only dist/.gitkeep is embedded.
		if present {
			t.Fatal("placeholder embed must not report index.html")
		}
	}
}

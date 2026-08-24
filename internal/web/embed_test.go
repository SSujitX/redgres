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

// TestAssetsPlaceholderTreeIsEmptyButUsable pins the placeholder behavior that
// TestAssetsAvailableWithoutFrontendBuild depends on: when dist/app is absent the
// root is still statable and readable, and it exposes no file. The tracked
// dist/.gitkeep must never become reachable.
func TestAssetsPlaceholderTreeIsEmptyButUsable(t *testing.T) {
	assets := emptyFS{}

	info, err := fs.Stat(assets, ".")
	if err != nil {
		t.Fatalf("stat root: %v", err)
	}
	if !info.IsDir() {
		t.Fatal("root is not a directory")
	}
	entries, err := fs.ReadDir(assets, ".")
	if err != nil {
		t.Fatalf("read root: %v", err)
	}
	if len(entries) != 0 {
		t.Fatalf("root exposes %d entries", len(entries))
	}
	for _, name := range []string{"index.html", ".gitkeep", "assets/app.js"} {
		if _, err := assets.Open(name); err == nil {
			t.Fatalf("placeholder tree served %q", name)
		}
	}
}

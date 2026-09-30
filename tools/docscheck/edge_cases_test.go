package main

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestRunSkipsInstalledDependencies(t *testing.T) {
	root := t.TempDir()
	deps := filepath.Join(root, "node_modules", "a-package")
	if err := os.MkdirAll(deps, 0o700); err != nil {
		t.Fatal(err)
	}
	writeTestFile(t, filepath.Join(root, "README.md"), "# Project\n")
	writeTestFile(t, filepath.Join(deps, "README.md"), "invalid dependency markdown")
	count, err := run(root)
	if err != nil || count != 1 {
		t.Fatalf("run = (%d, %v), want one project document", count, err)
	}
}

func TestRunCodeFenceDelimiters(t *testing.T) {
	tests := []struct {
		name, text string
		wantError  bool
	}{
		{"long outer fence", "````markdown\n```go\n[example](missing.md)\n```\n````\n", false},
		{"long closing fence", "```text\nexample\n`````\n", false},
		{"close has info", "```text\n```not-a-close\n", true},
		{"mixed markers", "```text\n~~~\n", true},
		{"short closing fence", "````text\n```\n", true},
		{"tilde outer fence", "~~~~text\n~~~\n[example](missing.md)\n~~~~\n", false},
		{"indented closing fence", "```text\n  ```\n", false},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			root := t.TempDir()
			writeTestFile(t, filepath.Join(root, "README.md"), "# Example\n\n"+test.text)
			_, err := run(root)
			if (err != nil) != test.wantError {
				t.Fatalf("run = %v, want error=%t", err, test.wantError)
			}
		})
	}
}

func TestRunDecodesLocalLinkOnce(t *testing.T) {
	root := t.TempDir()
	writeTestFile(t, filepath.Join(root, "literal%20name.txt"), "text\n")
	writeTestFile(t, filepath.Join(root, "README.md"), "# Links\n\n[file](literal%2520name.txt)\n")
	if _, err := run(root); err != nil {
		t.Fatal(err)
	}
}

func TestRunRejectsProtocolRelativeLink(t *testing.T) {
	root := t.TempDir()
	writeTestFile(t, filepath.Join(root, "README.md"), "# Links\n\n[site](//example.com)\n")
	if _, err := run(root); err == nil || !strings.Contains(err.Error(), "HTTPS") {
		t.Fatalf("run = %v, want explicit HTTPS requirement", err)
	}
}

func TestRunAcceptsAngleBracketLinkWithSpaces(t *testing.T) {
	root := t.TempDir()
	writeTestFile(t, filepath.Join(root, "file name.txt"), "text\n")
	writeTestFile(t, filepath.Join(root, "README.md"), "# Links\n\n[file](<file name.txt>)\n")
	if _, err := run(root); err != nil {
		t.Fatal(err)
	}
}

func TestRunAcceptsAngleBracketReferenceWithSpaces(t *testing.T) {
	root := t.TempDir()
	writeTestFile(t, filepath.Join(root, "file name.txt"), "text\n")
	writeTestFile(t, filepath.Join(root, "README.md"), "# Links\n\n[file][note]\n\n[note]: <file name.txt> \"A title\"\n")
	if _, err := run(root); err != nil {
		t.Fatal(err)
	}
}

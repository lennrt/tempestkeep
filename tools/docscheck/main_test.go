package main

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestRunAcceptsBoundedMarkdownAndLocalLinks(t *testing.T) {
	root := t.TempDir()
	writeTestFile(t, filepath.Join(root, "target.md"), "# Target\n")
	writeTestFile(t, filepath.Join(root, "README.md"), "# Project\n\nSee [target](target.md).\n")

	count, err := run(root)
	if err != nil {
		t.Fatal(err)
	}
	if count != 2 {
		t.Fatalf("file count = %d, want 2", count)
	}
}

func TestRunRejectsMarkdownDefects(t *testing.T) {
	tests := []struct {
		name string
		text string
		want string
	}{
		{name: "missing newline", text: "# Project", want: "file must end with a newline"},
		{name: "trailing whitespace", text: "# Project  \n", want: "trailing whitespace"},
		{name: "heading jump", text: "# Project\n\n### Detail\n", want: "heading level jumps"},
		{name: "open fence", text: "# Project\n\n```text\nvalue\n", want: "code fence is not closed"},
		{name: "missing link", text: "# Project\n\n[missing](missing.md)\n", want: "local link target does not exist"},
		{name: "insecure link", text: "# Project\n\n[site](http://example.com)\n", want: "external links must use HTTPS"},
		{name: "long prose", text: "# Project\n\n" + strings.Repeat("x", maxLineBytes+1) + "\n", want: "line exceeds"},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			root := t.TempDir()
			writeTestFile(t, filepath.Join(root, "README.md"), test.text)
			_, err := run(root)
			if err == nil || !strings.Contains(err.Error(), test.want) {
				t.Fatalf("run error = %v, want %q", err, test.want)
			}
		})
	}
}

func TestRunRejectsWrongCaseLocalLink(t *testing.T) {
	root := t.TempDir()
	writeTestFile(t, filepath.Join(root, "target.md"), "# Target\n")
	writeTestFile(t, filepath.Join(root, "README.md"), "# Project\n\n[target](TARGET.md)\n")
	_, err := run(root)
	if err == nil || !strings.Contains(err.Error(), "local link target does not exist") {
		t.Fatalf("run error = %v, want case-sensitive link failure", err)
	}
}

func TestRunIgnoresLongTableAndCodeLines(t *testing.T) {
	root := t.TempDir()
	long := strings.Repeat("x", maxLineBytes+20)
	text := "# Project\n\n| Column |\n|---|\n| " + long + " |\n\n```text\n" + long + "\n```\n"
	writeTestFile(t, filepath.Join(root, "README.md"), text)
	if _, err := run(root); err != nil {
		t.Fatal(err)
	}
}

func writeTestFile(t *testing.T, path, content string) {
	t.Helper()
	if err := os.WriteFile(path, []byte(content), 0o600); err != nil {
		t.Fatal(err)
	}
}

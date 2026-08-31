// Command docscheck validates repository Markdown without network access.
package main

import (
	"errors"
	"flag"
	"fmt"
	"io"
	"net/url"
	"os"
	"path/filepath"
	"regexp"
	"slices"
	"strings"
	"unicode/utf8"
)

const (
	maxMarkdownFiles   = 1_000
	maxMarkdownBytes   = 2 << 20
	maxDirectoryItems  = 10_000
	maxRepositoryItems = 100_000
	maxDirectoryDepth  = 64
	maxLineBytes       = 100
	maxFindings        = 200
)

var (
	errMarkdownChanged    = errors.New("markdown file changed during validation")
	errMarkdownNotRegular = errors.New("markdown path is not a regular file")
	errMarkdownTooLarge   = errors.New("markdown file exceeds the size limit")

	headingPattern       = regexp.MustCompile(`^(#{1,6})[ \t]+\S`)
	inlineLinkPattern    = regexp.MustCompile(`!?\[[^\]\n]*\]\(([^)\n]+)\)`)
	referenceLinkPattern = regexp.MustCompile(`^\[[^\]\n]+\]:[ \t]+(\S+)`)
)

type finding struct {
	path string
	line int
	kind string
}

type checker struct {
	root     string
	findings []finding
}

func main() {
	root := flag.String("root", ".", "repository root")
	flag.Parse()

	count, err := run(*root)
	if err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}
	fmt.Printf("documentation checks passed (%d Markdown files)\n", count)
}

func run(root string) (int, error) {
	root, err := filepath.Abs(root)
	if err != nil {
		return 0, fmt.Errorf("resolve repository root: %w", err)
	}
	root, err = filepath.EvalSymlinks(root)
	if err != nil {
		return 0, fmt.Errorf("resolve repository root identity: %w", err)
	}

	paths, err := markdownPaths(root)
	if err != nil {
		return 0, err
	}
	c := checker{root: root}
	for _, path := range paths {
		c.checkFile(path)
		if len(c.findings) >= maxFindings {
			break
		}
	}
	if len(c.findings) == 0 {
		return len(paths), nil
	}

	slices.SortFunc(c.findings, func(a, b finding) int {
		if byPath := strings.Compare(a.path, b.path); byPath != 0 {
			return byPath
		}
		if a.line < b.line {
			return -1
		}
		if a.line > b.line {
			return 1
		}
		return strings.Compare(a.kind, b.kind)
	})
	var errs []error
	for _, item := range c.findings {
		errs = append(errs, fmt.Errorf("%s:%d: %s", item.path, item.line, item.kind))
	}
	if len(c.findings) >= maxFindings {
		errs = append(errs, errors.New("documentation finding limit reached"))
	}
	return len(paths), errors.Join(errs...)
}

func markdownPaths(root string) ([]string, error) {
	paths := make([]string, 0, 32)
	itemCount := 0
	var walk func(string, int) error
	walk = func(dir string, depth int) error {
		if depth > maxDirectoryDepth {
			return fmt.Errorf("repository exceeds the directory depth limit of %d", maxDirectoryDepth)
		}
		entries, err := boundedDirectoryEntries(dir)
		if err != nil {
			return err
		}
		for _, entry := range entries {
			itemCount++
			if itemCount > maxRepositoryItems {
				return fmt.Errorf("repository exceeds %d filesystem items", maxRepositoryItems)
			}
			path := filepath.Join(dir, entry.Name())
			if entry.IsDir() {
				switch entry.Name() {
				case ".git", "bin", "dist", "vendor":
					continue
				}
				if err := walk(path, depth+1); err != nil {
					return err
				}
				continue
			}
			if !strings.EqualFold(filepath.Ext(entry.Name()), ".md") {
				continue
			}
			if entry.Mode()&os.ModeSymlink != 0 {
				return fmt.Errorf("%s: markdown file must not be a symbolic link", displayPath(root, path))
			}
			if !entry.Mode().IsRegular() {
				return fmt.Errorf("%s: markdown path is not a regular file", displayPath(root, path))
			}
			if entry.Size() > maxMarkdownBytes {
				return fmt.Errorf("%s: markdown file exceeds %d bytes", displayPath(root, path), maxMarkdownBytes)
			}
			paths = append(paths, path)
			if len(paths) > maxMarkdownFiles {
				return fmt.Errorf("repository exceeds %d Markdown files", maxMarkdownFiles)
			}
		}
		return nil
	}
	err := walk(root, 0)
	if err != nil {
		return nil, fmt.Errorf("walk Markdown files: %w", err)
	}
	slices.Sort(paths)
	return paths, nil
}

func (c *checker) checkFile(path string) {
	data, err := readMarkdown(path)
	if err != nil {
		switch {
		case errors.Is(err, errMarkdownChanged):
			c.add(path, 1, errMarkdownChanged.Error())
		case errors.Is(err, errMarkdownNotRegular):
			c.add(path, 1, errMarkdownNotRegular.Error())
		case errors.Is(err, errMarkdownTooLarge):
			c.add(path, 1, errMarkdownTooLarge.Error())
		default:
			c.add(path, 1, "read failed")
		}
		return
	}
	if !utf8.Valid(data) {
		c.add(path, 1, "file is not valid UTF-8")
		return
	}
	if len(data) == 0 || data[len(data)-1] != '\n' {
		c.add(path, 1, "file must end with a newline")
	}
	text := string(data)
	if strings.ContainsRune(text, '\r') {
		c.add(path, 1, "file must use LF line endings")
	}

	inFence := false
	fence := ""
	previousHeading := 0
	lineNumber := 0
	for line := range strings.SplitSeq(text, "\n") {
		lineNumber++
		trimmed := strings.TrimSpace(line)
		if strings.HasSuffix(line, " ") || strings.HasSuffix(line, "\t") {
			c.add(path, lineNumber, "trailing whitespace")
		}
		if marker, ok := fenceMarker(trimmed); ok {
			if !inFence {
				inFence, fence = true, marker
			} else if marker == fence {
				inFence, fence = false, ""
			}
			continue
		}
		if inFence {
			continue
		}
		if len(line) > maxLineBytes && !strings.HasPrefix(trimmed, "|") {
			c.add(path, lineNumber, fmt.Sprintf("line exceeds %d bytes", maxLineBytes))
		}
		if match := headingPattern.FindStringSubmatch(line); match != nil {
			level := len(match[1])
			if previousHeading != 0 && level > previousHeading+1 {
				c.add(path, lineNumber, "heading level jumps by more than one")
			}
			previousHeading = level
		}
		for _, match := range inlineLinkPattern.FindAllStringSubmatch(line, -1) {
			c.checkLink(path, lineNumber, firstLinkField(match[1]))
		}
		if match := referenceLinkPattern.FindStringSubmatch(line); match != nil {
			c.checkLink(path, lineNumber, firstLinkField(match[1]))
		}
	}
	if inFence {
		c.add(path, lineNumber, "code fence is not closed")
	}
}

func fenceMarker(line string) (string, bool) {
	switch {
	case strings.HasPrefix(line, "```"):
		return "```", true
	case strings.HasPrefix(line, "~~~"):
		return "~~~", true
	default:
		return "", false
	}
}

func firstLinkField(raw string) string {
	fields := strings.Fields(raw)
	if len(fields) == 0 {
		return ""
	}
	return strings.Trim(fields[0], "<>")
}

func (c *checker) checkLink(source string, line int, target string) {
	if target == "" || strings.HasPrefix(target, "#") || strings.HasPrefix(target, "mailto:") {
		return
	}
	parsed, err := url.Parse(target)
	if err != nil {
		c.add(source, line, "link target is invalid")
		return
	}
	if parsed.IsAbs() {
		if parsed.Scheme != "https" || parsed.User != nil || parsed.Host == "" {
			c.add(source, line, "external links must use HTTPS without user information")
		}
		return
	}
	pathPart := parsed.Path
	if pathPart == "" {
		return
	}
	decoded, err := url.PathUnescape(pathPart)
	if err != nil || filepath.IsAbs(decoded) {
		c.add(source, line, "local link path is invalid")
		return
	}
	resolved := filepath.Clean(filepath.Join(filepath.Dir(source), filepath.FromSlash(decoded)))
	relative, err := filepath.Rel(c.root, resolved)
	if err != nil || relative == ".." || strings.HasPrefix(relative, ".."+string(filepath.Separator)) {
		c.add(source, line, "local link escapes the repository")
		return
	}
	if _, err := os.Stat(resolved); err != nil {
		c.add(source, line, "local link target does not exist")
		return
	}
	exact, err := pathHasExactCase(c.root, relative)
	if err != nil || !exact {
		c.add(source, line, "local link target does not exist with exact case")
	}
}

func (c *checker) add(path string, line int, kind string) {
	if len(c.findings) >= maxFindings {
		return
	}
	relative, err := filepath.Rel(c.root, path)
	if err != nil {
		relative = path
	}
	c.findings = append(c.findings, finding{path: filepath.ToSlash(relative), line: line, kind: kind})
}

func readMarkdown(path string) (_ []byte, err error) {
	before, err := os.Lstat(path)
	if err != nil {
		return nil, err
	}
	if !before.Mode().IsRegular() {
		return nil, errMarkdownNotRegular
	}
	if before.Size() > maxMarkdownBytes {
		return nil, errMarkdownTooLarge
	}
	file, err := os.Open(path)
	if err != nil {
		return nil, err
	}
	defer func() { err = errors.Join(err, file.Close()) }()
	after, err := file.Stat()
	if err != nil {
		return nil, err
	}
	if !after.Mode().IsRegular() {
		return nil, errMarkdownNotRegular
	}
	if !os.SameFile(before, after) {
		return nil, errMarkdownChanged
	}
	data, err := io.ReadAll(io.LimitReader(file, maxMarkdownBytes+1))
	if err != nil {
		return nil, err
	}
	if len(data) > maxMarkdownBytes {
		return nil, errMarkdownTooLarge
	}
	return data, nil
}

func boundedDirectoryEntries(path string) (_ []os.FileInfo, err error) {
	dir, err := os.Open(path)
	if err != nil {
		return nil, err
	}
	defer func() { err = errors.Join(err, dir.Close()) }()
	entries, readErr := dir.Readdir(maxDirectoryItems + 1)
	if readErr != nil && !errors.Is(readErr, io.EOF) {
		return nil, readErr
	}
	if len(entries) > maxDirectoryItems {
		return nil, fmt.Errorf("directory exceeds %d entries", maxDirectoryItems)
	}
	slices.SortFunc(entries, func(a, b os.FileInfo) int {
		return strings.Compare(a.Name(), b.Name())
	})
	return entries, nil
}

func pathHasExactCase(root, relative string) (bool, error) {
	if relative == "." {
		return true, nil
	}
	current := root
	for part := range strings.SplitSeq(relative, string(filepath.Separator)) {
		entries, err := boundedDirectoryEntries(current)
		if err != nil {
			return false, err
		}
		found := false
		for _, entry := range entries {
			if entry.Name() == part {
				found = true
				break
			}
		}
		if !found {
			return false, nil
		}
		current = filepath.Join(current, part)
	}
	return true, nil
}

func displayPath(root, path string) string {
	relative, err := filepath.Rel(root, path)
	if err != nil {
		return filepath.Base(path)
	}
	return filepath.ToSlash(relative)
}

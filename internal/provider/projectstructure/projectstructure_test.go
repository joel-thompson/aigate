package projectstructure

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func makeTree(t *testing.T, root string, paths []string) {
	t.Helper()
	for _, p := range paths {
		full := filepath.Join(root, p)
		if strings.HasSuffix(p, "/") {
			if err := os.MkdirAll(full, 0755); err != nil {
				t.Fatal(err)
			}
		} else {
			if err := os.MkdirAll(filepath.Dir(full), 0755); err != nil {
				t.Fatal(err)
			}
			if err := os.WriteFile(full, []byte(""), 0644); err != nil {
				t.Fatal(err)
			}
		}
	}
}

func TestRun_BasicTree(t *testing.T) {
	root := t.TempDir()
	makeTree(t, root, []string{
		"src/main.go",
		"src/lib.go",
		"README.md",
		"go.mod",
	})

	p := &Provider{Root: root, MaxDepth: 3}
	result, err := p.Run(context.Background())
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if result.Label != "Project structure" {
		t.Errorf("expected label %q, got %q", "Project structure", result.Label)
	}

	lines := strings.Split(result.Output, "\n")
	if lines[0] != "." {
		t.Errorf("expected first line to be '.', got %q", lines[0])
	}

	output := result.Output
	if !strings.Contains(output, "src") {
		t.Errorf("expected output to contain 'src', got:\n%s", output)
	}
	if !strings.Contains(output, "main.go") {
		t.Errorf("expected output to contain 'main.go', got:\n%s", output)
	}
	if !strings.Contains(output, "README.md") {
		t.Errorf("expected output to contain 'README.md', got:\n%s", output)
	}
	if !strings.Contains(output, "go.mod") {
		t.Errorf("expected output to contain 'go.mod', got:\n%s", output)
	}
}

func TestRun_RespectsGitignore(t *testing.T) {
	root := t.TempDir()
	makeTree(t, root, []string{
		"src/main.go",
		"build/output.bin",
		"debug.log",
		"important.txt",
	})
	if err := os.WriteFile(filepath.Join(root, ".gitignore"), []byte("build/\n*.log\n"), 0644); err != nil {
		t.Fatal(err)
	}

	p := &Provider{Root: root, MaxDepth: 3}
	result, err := p.Run(context.Background())
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	output := result.Output
	if strings.Contains(output, "build") {
		t.Errorf("expected 'build' to be excluded by .gitignore, got:\n%s", output)
	}
	if strings.Contains(output, "debug.log") {
		t.Errorf("expected 'debug.log' to be excluded by .gitignore, got:\n%s", output)
	}
	if !strings.Contains(output, "main.go") {
		t.Errorf("expected 'main.go' to be present, got:\n%s", output)
	}
	if !strings.Contains(output, "important.txt") {
		t.Errorf("expected 'important.txt' to be present, got:\n%s", output)
	}
}

func TestRun_MaxDepthEnforced(t *testing.T) {
	root := t.TempDir()
	makeTree(t, root, []string{
		"a/b/c/d/deep.txt",
	})

	p := &Provider{Root: root, MaxDepth: 2}
	result, err := p.Run(context.Background())
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	output := result.Output
	if !strings.Contains(output, "a") {
		t.Errorf("expected depth-1 dir 'a', got:\n%s", output)
	}
	if !strings.Contains(output, "b") {
		t.Errorf("expected depth-2 dir 'b', got:\n%s", output)
	}
	if strings.Contains(output, "c") {
		t.Errorf("expected depth-3 dir 'c' to be excluded at max_depth=2, got:\n%s", output)
	}
	if strings.Contains(output, "deep.txt") {
		t.Errorf("expected 'deep.txt' to be excluded at max_depth=2, got:\n%s", output)
	}
}

func TestRun_DefaultMaxDepth(t *testing.T) {
	root := t.TempDir()
	makeTree(t, root, []string{
		"a/b/level2.txt",
		"a/b/c/d/level4.txt",
	})

	p := &Provider{Root: root}
	result, err := p.Run(context.Background())
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	output := result.Output
	if !strings.Contains(output, "level2.txt") {
		t.Errorf("expected depth-2 file to be included with default max_depth, got:\n%s", output)
	}
	if strings.Contains(output, "level4.txt") {
		t.Errorf("expected depth-4 file to be excluded with default max_depth=3, got:\n%s", output)
	}
}

func TestRun_SkipsAlwaysSkipDirs(t *testing.T) {
	root := t.TempDir()
	makeTree(t, root, []string{
		".git/config",
		"node_modules/pkg/index.js",
		"vendor/lib/lib.go",
		"__pycache__/module.pyc",
		".venv/bin/python",
		"src/main.go",
	})

	p := &Provider{Root: root, MaxDepth: 3}
	result, err := p.Run(context.Background())
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	output := result.Output
	for _, dir := range []string{".git", "node_modules", "vendor", "__pycache__", ".venv"} {
		if strings.Contains(output, dir) {
			t.Errorf("expected %q to be skipped, got:\n%s", dir, output)
		}
	}
	if !strings.Contains(output, "src") {
		t.Errorf("expected 'src' to be present, got:\n%s", output)
	}
}

func TestRun_EmptyDirectory(t *testing.T) {
	root := t.TempDir()
	makeTree(t, root, []string{
		"empty/",
		"notempty/file.txt",
	})

	p := &Provider{Root: root, MaxDepth: 3}
	result, err := p.Run(context.Background())
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	output := result.Output
	if !strings.Contains(output, "empty") {
		t.Errorf("expected empty directory to appear in output, got:\n%s", output)
	}
	if !strings.Contains(output, "notempty") {
		t.Errorf("expected non-empty directory to appear, got:\n%s", output)
	}
}

func TestRun_TreeConnectors(t *testing.T) {
	root := t.TempDir()
	makeTree(t, root, []string{
		"aaa.txt",
		"zzz.txt",
	})

	p := &Provider{Root: root, MaxDepth: 1}
	result, err := p.Run(context.Background())
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	lines := strings.Split(result.Output, "\n")
	if len(lines) != 3 {
		t.Fatalf("expected 3 lines (root + 2 files), got %d:\n%s", len(lines), result.Output)
	}
	if !strings.HasPrefix(lines[1], "├── ") {
		t.Errorf("expected non-last item to use '├── ', got %q", lines[1])
	}
	if !strings.HasPrefix(lines[2], "└── ") {
		t.Errorf("expected last item to use '└── ', got %q", lines[2])
	}
}

func TestRun_DirectoriesSortedBeforeFiles(t *testing.T) {
	root := t.TempDir()
	makeTree(t, root, []string{
		"zebra.txt",
		"alpha/file.txt",
	})

	p := &Provider{Root: root, MaxDepth: 1}
	result, err := p.Run(context.Background())
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	lines := strings.Split(result.Output, "\n")
	dirIdx := -1
	fileIdx := -1
	for i, l := range lines {
		if strings.Contains(l, "alpha") {
			dirIdx = i
		}
		if strings.Contains(l, "zebra") {
			fileIdx = i
		}
	}
	if dirIdx == -1 || fileIdx == -1 {
		t.Fatalf("expected both entries, got:\n%s", result.Output)
	}
	if dirIdx > fileIdx {
		t.Errorf("expected directory before file, got dir at %d, file at %d", dirIdx, fileIdx)
	}
}

func TestRun_NoGitignoreFile(t *testing.T) {
	root := t.TempDir()
	makeTree(t, root, []string{
		"file.txt",
	})

	p := &Provider{Root: root, MaxDepth: 3}
	result, err := p.Run(context.Background())
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !strings.Contains(result.Output, "file.txt") {
		t.Errorf("expected file to appear without .gitignore, got:\n%s", result.Output)
	}
}

func TestRun_RootNotFound(t *testing.T) {
	p := &Provider{Root: "/nonexistent/path/that/does/not/exist", MaxDepth: 3}
	_, err := p.Run(context.Background())
	if err == nil {
		t.Fatal("expected error for nonexistent root")
	}
	if !strings.Contains(err.Error(), "reading directory") {
		t.Errorf("expected 'reading directory' in error, got %q", err.Error())
	}
}

func TestRun_BreadthCap(t *testing.T) {
	root := t.TempDir()
	var paths []string
	for i := 0; i < maxEntriesPerDir+10; i++ {
		paths = append(paths, fmt.Sprintf("file_%03d.txt", i))
	}
	makeTree(t, root, paths)

	p := &Provider{Root: root, MaxDepth: 1}
	result, err := p.Run(context.Background())
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	lines := strings.Split(result.Output, "\n")
	lastLine := lines[len(lines)-1]
	if !strings.Contains(lastLine, "... (10 more)") {
		t.Errorf("expected truncation indicator '... (10 more)', got last line: %q", lastLine)
	}

	outputLines := len(lines)
	// 1 root line + maxEntriesPerDir entries + 1 truncation line
	expected := 1 + maxEntriesPerDir + 1
	if outputLines != expected {
		t.Errorf("expected %d lines, got %d", expected, outputLines)
	}
}

func TestRun_CancelledContext(t *testing.T) {
	root := t.TempDir()
	makeTree(t, root, []string{
		"a/b/deep.txt",
		"c/d/other.txt",
	})

	ctx, cancel := context.WithCancel(context.Background())
	cancel()

	p := &Provider{Root: root, MaxDepth: 3}
	result, err := p.Run(ctx)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	lines := strings.Split(result.Output, "\n")
	if len(lines) > 3 {
		t.Errorf("expected cancelled context to stop early, got %d lines:\n%s", len(lines), result.Output)
	}
}

package projectstructure

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"

	ignore "github.com/sabhiram/go-gitignore"

	"github.com/joelthompson/aigate/internal/provider"
)

const (
	DefaultMaxDepth    = 3
	maxEntriesPerDir   = 50
)

var alwaysSkip = map[string]bool{
	".git":         true,
	"node_modules": true,
	"vendor":       true,
	"__pycache__":  true,
	".venv":        true,
}

type Provider struct {
	Root     string
	MaxDepth int
}

func (p *Provider) Run(ctx context.Context) (*provider.Result, error) {
	root := p.Root
	if root == "" {
		var err error
		root, err = os.Getwd()
		if err != nil {
			return nil, fmt.Errorf("getting working directory: %w", err)
		}
	}

	maxDepth := p.MaxDepth
	if maxDepth <= 0 {
		maxDepth = DefaultMaxDepth
	}

	gi := loadGitignore(root)

	entries, err := os.ReadDir(root)
	if err != nil {
		return nil, fmt.Errorf("reading directory %s: %w", root, err)
	}

	var lines []string
	lines = append(lines, ".")
	buildTree(ctx, root, root, "", maxDepth, 0, gi, entries, &lines)

	return &provider.Result{
		Output: strings.Join(lines, "\n"),
		Label:  "Project structure",
	}, nil
}

func loadGitignore(root string) *ignore.GitIgnore {
	path := filepath.Join(root, ".gitignore")
	gi, err := ignore.CompileIgnoreFile(path)
	if err != nil {
		return nil
	}
	return gi
}

func buildTree(ctx context.Context, root, dir, prefix string, maxDepth, depth int, gi *ignore.GitIgnore, entries []os.DirEntry, lines *[]string) {
	if depth >= maxDepth {
		return
	}
	if ctx.Err() != nil {
		return
	}

	visible := filterEntries(root, dir, entries, gi)

	sort.Slice(visible, func(i, j int) bool {
		di, dj := visible[i].IsDir(), visible[j].IsDir()
		if di != dj {
			return di
		}
		return visible[i].Name() < visible[j].Name()
	})

	originalCount := len(visible)
	truncated := originalCount > maxEntriesPerDir
	if truncated {
		visible = visible[:maxEntriesPerDir]
	}

	for i, e := range visible {
		isLast := i == len(visible)-1 && !truncated
		connector := "├── "
		if isLast {
			connector = "└── "
		}
		*lines = append(*lines, prefix+connector+e.Name())

		if e.IsDir() {
			childPrefix := prefix + "│   "
			if isLast {
				childPrefix = prefix + "    "
			}
			childEntries, err := os.ReadDir(filepath.Join(dir, e.Name()))
			if err != nil {
				continue
			}
			buildTree(ctx, root, filepath.Join(dir, e.Name()), childPrefix, maxDepth, depth+1, gi, childEntries, lines)
		}
	}

	if truncated {
		*lines = append(*lines, fmt.Sprintf("%s└── ... (%d more)", prefix, originalCount-maxEntriesPerDir))
	}
}

func filterEntries(root, dir string, entries []os.DirEntry, gi *ignore.GitIgnore) []os.DirEntry {
	var visible []os.DirEntry
	for _, e := range entries {
		name := e.Name()
		if alwaysSkip[name] {
			continue
		}
		if gi != nil {
			rel, err := filepath.Rel(root, filepath.Join(dir, name))
			if err == nil {
				if e.IsDir() {
					rel += "/"
				}
				if gi.MatchesPath(rel) {
					continue
				}
			}
		}
		visible = append(visible, e)
	}
	return visible
}

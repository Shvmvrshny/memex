package memex

import (
	"flag"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
)

// RunIndex is the CLI handler for `memex index [--changed] [--path <repo>]`.
func RunIndex(args []string) {
	fs := flag.NewFlagSet("index", flag.ExitOnError)
	changed := fs.Bool("changed", false, "index changed Go files only")
	repoPath := fs.String("path", ".", "repository path to index")
	_ = fs.Parse(args)

	root, err := filepath.Abs(*repoPath)
	if err != nil {
		fmt.Fprintf(os.Stderr, "index: resolve repo path: %v\n", err)
		os.Exit(1)
	}

	commitHash := gitHeadCommit(root)
	if strings.TrimSpace(commitHash) == "" {
		commitHash = "working-tree"
	}

	var files []string
	if *changed {
		files, err = listChangedGoFiles(root)
	} else {
		files, err = listAllGoFiles(root)
	}
	if err != nil {
		fmt.Fprintf(os.Stderr, "index: list files: %v\n", err)
		os.Exit(1)
	}
	if len(files) == 0 {
		fmt.Println("memex index: no Go files to process")
		return
	}

	cfg := LoadConfig()
	kg, err := NewKnowledgeGraph(cfg.KGPath)
	if err != nil {
		fmt.Fprintf(os.Stderr, "index: init knowledge graph: %v\n", err)
		os.Exit(1)
	}
	defer kg.db.Close()
	if err := kg.Init(); err != nil {
		fmt.Fprintf(os.Stderr, "index: init knowledge graph schema: %v\n", err)
		os.Exit(1)
	}

	indexer := NewCodeIndexer(root, commitHash)
	facts, err := indexer.ExtractFactsForFiles(files)
	if err != nil {
		fmt.Fprintf(os.Stderr, "index: extract facts: %v\n", err)
		os.Exit(1)
	}

	expiredCount := int64(0)
	// Cleanup legacy index noise from ephemeral worktree paths.
	if n, err := kg.ExpireActiveFactsByPrefix(".worktrees/"); err == nil {
		expiredCount += n
	}
	fileSet := map[string]struct{}{}
	for _, f := range facts {
		if f.FilePath == "" {
			continue
		}
		fileSet[f.FilePath] = struct{}{}
	}
	for filePath := range fileSet {
		n, err := kg.ExpireActiveFactsByFile(filePath)
		if err != nil {
			fmt.Fprintf(os.Stderr, "index: expire facts for %s: %v\n", filePath, err)
			os.Exit(1)
		}
		expiredCount += n
	}

	inserted := 0
	for _, fact := range facts {
		if _, err := kg.RecordFactScoped(fact, false); err != nil {
			fmt.Fprintf(os.Stderr, "index: record fact (%s %s %s): %v\n", fact.Subject, fact.Predicate, fact.Object, err)
			os.Exit(1)
		}
		inserted++
	}

	mode := "full"
	if *changed {
		mode = "changed"
	}
	fmt.Printf("memex index: mode=%s files=%d expired=%d inserted=%d commit=%s\n",
		mode, len(fileSet), expiredCount, inserted, commitHash)
}

func gitHeadCommit(repoRoot string) string {
	cmd := exec.Command("git", "rev-parse", "HEAD")
	cmd.Dir = repoRoot
	out, err := cmd.Output()
	if err != nil {
		return ""
	}
	return strings.TrimSpace(string(out))
}

func listChangedGoFiles(repoRoot string) ([]string, error) {
	commands := [][]string{
		{"git", "diff", "--name-only", "HEAD"},
		{"git", "diff", "--name-only", "--cached"},
		{"git", "ls-files", "--others", "--exclude-standard"},
	}
	seen := map[string]struct{}{}
	var out []string

	for _, argv := range commands {
		cmd := exec.Command(argv[0], argv[1:]...)
		cmd.Dir = repoRoot
		b, err := cmd.Output()
		if err != nil {
			// Continue for non-git contexts.
			continue
		}
		for _, line := range strings.Split(string(b), "\n") {
			line = strings.TrimSpace(line)
			if line == "" || !strings.HasSuffix(line, ".go") {
				continue
			}
			if strings.Contains(filepath.ToSlash(line), "/.worktrees/") || strings.HasPrefix(filepath.ToSlash(line), ".worktrees/") {
				continue
			}
			abs := filepath.Join(repoRoot, filepath.FromSlash(line))
			if _, err := os.Stat(abs); err != nil {
				continue
			}
			if _, ok := seen[abs]; ok {
				continue
			}
			seen[abs] = struct{}{}
			out = append(out, abs)
		}
	}
	return out, nil
}

func listAllGoFiles(repoRoot string) ([]string, error) {
	var out []string
	err := filepath.WalkDir(repoRoot, func(path string, d os.DirEntry, err error) error {
		if err != nil {
			return err
		}
		if d.IsDir() {
			name := d.Name()
			if name == ".git" || name == ".worktrees" || name == "vendor" || name == "node_modules" {
				return filepath.SkipDir
			}
			return nil
		}
		if strings.HasSuffix(path, ".go") {
			out = append(out, path)
		}
		return nil
	})
	return out, err
}

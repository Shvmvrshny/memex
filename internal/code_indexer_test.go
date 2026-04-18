package memex

import (
	"os"
	"path/filepath"
	"testing"
)

func TestCodeIndexer_ExtractFactsForFiles(t *testing.T) {
	root := t.TempDir()
	mustWriteFile(t, filepath.Join(root, "go.mod"), "module example.com/repo\n\ngo 1.26.1\n")
	mustWriteFile(t, filepath.Join(root, "pkg", "a.go"), `package pkg

import "fmt"

type S struct{}

func A() {
	B()
	fmt.Println("x")
	var s S
	s.M()
}

func B() {}

func (S) M() {}
`)
	mustWriteFile(t, filepath.Join(root, "pkg", "a_test.go"), `package pkg

import "testing"

func TestA(t *testing.T) {
	A()
}
`)

	indexer := NewCodeIndexer(root, "deadbeef")
	facts, err := indexer.ExtractFactsForFiles([]string{
		filepath.Join(root, "pkg", "a.go"),
		filepath.Join(root, "pkg", "a_test.go"),
	})
	if err != nil {
		t.Fatalf("ExtractFactsForFiles: %v", err)
	}
	if len(facts) == 0 {
		t.Fatal("expected facts, got none")
	}

	assertHasFact(t, facts, "pkg/a.go", PredicateContainsFunction, "example.com/repo/pkg::A")
	assertHasFact(t, facts, "pkg/a.go", PredicateContainsFunction, "example.com/repo/pkg::B")
	assertHasFact(t, facts, "pkg/a.go", PredicateContainsFunction, "example.com/repo/pkg::S.M")
	assertHasFact(t, facts, "example.com/repo/pkg::A", PredicateCalls, "example.com/repo/pkg::B")
	assertHasFact(t, facts, "example.com/repo/pkg::A", PredicateCalls, "fmt::Println")
	assertHasFact(t, facts, "example.com/repo/pkg::A", PredicateCallsUnresolved, "s.M")
	assertHasFact(t, facts, "example.com/repo/pkg", PredicateDependsOn, "fmt")
	assertHasFact(t, facts, "pkg/a_test.go", PredicateTestOf, "pkg/a.go")
}

func assertHasFact(t *testing.T, facts []Fact, subject, predicate, object string) {
	t.Helper()
	for _, f := range facts {
		if f.Subject == subject && f.Predicate == predicate && f.Object == object {
			return
		}
	}
	t.Fatalf("missing fact: %s %s %s", subject, predicate, object)
}

func mustWriteFile(t *testing.T, path, content string) {
	t.Helper()
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		t.Fatalf("MkdirAll(%s): %v", path, err)
	}
	if err := os.WriteFile(path, []byte(content), 0o644); err != nil {
		t.Fatalf("WriteFile(%s): %v", path, err)
	}
}

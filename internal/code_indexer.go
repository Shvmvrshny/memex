package memex

import (
	"bytes"
	"fmt"
	"go/ast"
	"go/format"
	"go/parser"
	"go/token"
	"os"
	"os/exec"
	"path/filepath"
	"sort"
	"strings"
)

const (
	PredicateContainsFunction = "contains"
	PredicateCalls            = "calls"
	PredicateCallsUnresolved  = "calls_unresolved"
	PredicateDependsOn        = "depends_on"
	PredicateTestOf           = "test_of"
)

// CodeIndexer extracts deterministic code structure into KG facts.
// Phase 1 scope:
// - file -> contains -> function
// - function -> calls -> function (direct/static only)
// - package -> depends_on -> package
// - function -> calls_unresolved -> symbol (for interface/dynamic/unknown dispatch)
type CodeIndexer struct {
	repoRoot   string
	commitHash string
}

func NewCodeIndexer(repoRoot, commitHash string) *CodeIndexer {
	return &CodeIndexer{
		repoRoot:   repoRoot,
		commitHash: commitHash,
	}
}

func (c *CodeIndexer) ExtractFactsForFiles(files []string) ([]Fact, error) {
	fset := token.NewFileSet()
	parsedByFile := map[string]*ast.File{}
	packageIDByFile := map[string]string{}
	functionsByPackage := map[string]map[string]struct{}{}
	importsByFile := map[string]map[string]string{}
	var allFacts []Fact

	for _, file := range files {
		abs := file
		if !filepath.IsAbs(abs) {
			abs = filepath.Join(c.repoRoot, file)
		}
		src, err := parser.ParseFile(fset, abs, nil, parser.ParseComments)
		if err != nil {
			return nil, fmt.Errorf("parse %s: %w", abs, err)
		}
		parsedByFile[abs] = src

		pkgID, err := c.resolvePackageID(filepath.Dir(abs))
		if err != nil {
			return nil, err
		}
		packageIDByFile[abs] = pkgID
		if _, ok := functionsByPackage[pkgID]; !ok {
			functionsByPackage[pkgID] = map[string]struct{}{}
		}

		importsByFile[abs] = collectImports(src)

		rel := c.toRepoRelative(abs)
		if strings.HasSuffix(rel, "_test.go") {
			baseCandidate := strings.TrimSuffix(rel, "_test.go") + ".go"
			if _, err := os.Stat(filepath.Join(c.repoRoot, filepath.FromSlash(baseCandidate))); err == nil {
				allFacts = append(allFacts, Fact{
					Subject:    rel,
					Predicate:  PredicateTestOf,
					Object:     baseCandidate,
					Source:     "ast",
					FilePath:   rel,
					CommitHash: c.commitHash,
					Confidence: 1,
				})
			}
		}
		for _, decl := range src.Decls {
			fn, ok := decl.(*ast.FuncDecl)
			if !ok || fn.Name == nil {
				continue
			}
			fnName := qualifiedFunctionName(fn)
			functionsByPackage[pkgID][fnName] = struct{}{}
			fnID := fmt.Sprintf("%s::%s", pkgID, fnName)
			allFacts = append(allFacts, Fact{
				Subject:    rel,
				Predicate:  PredicateContainsFunction,
				Object:     fnID,
				Source:     "ast",
				FilePath:   rel,
				CommitHash: c.commitHash,
				Confidence: 1,
			})
		}

		for _, importPath := range importsByFile[abs] {
			allFacts = append(allFacts, Fact{
				Subject:    pkgID,
				Predicate:  PredicateDependsOn,
				Object:     importPath,
				Source:     "ast",
				FilePath:   rel,
				CommitHash: c.commitHash,
				Confidence: 1,
			})
		}
	}

	for abs, src := range parsedByFile {
		pkgID := packageIDByFile[abs]
		knownFns := functionsByPackage[pkgID]
		imports := importsByFile[abs]
		rel := c.toRepoRelative(abs)

		for _, decl := range src.Decls {
			fn, ok := decl.(*ast.FuncDecl)
			if !ok || fn.Body == nil || fn.Name == nil {
				continue
			}
			caller := fmt.Sprintf("%s::%s", pkgID, qualifiedFunctionName(fn))

			ast.Inspect(fn.Body, func(n ast.Node) bool {
				call, ok := n.(*ast.CallExpr)
				if !ok {
					return true
				}

				predicate := PredicateCalls
				target := ""
				meta := ""

				switch fun := call.Fun.(type) {
				case *ast.Ident:
					if _, exists := knownFns[fun.Name]; exists {
						target = fmt.Sprintf("%s::%s", pkgID, fun.Name)
					} else {
						predicate = PredicateCallsUnresolved
						target = fun.Name
						meta = `{"reason":"unresolved_ident"}`
					}
				case *ast.SelectorExpr:
					if ident, ok := fun.X.(*ast.Ident); ok {
						if imp, ok := imports[ident.Name]; ok {
							target = fmt.Sprintf("%s::%s", imp, fun.Sel.Name)
						} else {
							predicate = PredicateCallsUnresolved
							target = exprString(fset, fun)
							meta = `{"reason":"dynamic_selector"}`
						}
					} else {
						predicate = PredicateCallsUnresolved
						target = exprString(fset, fun)
						meta = `{"reason":"non_ident_selector"}`
					}
				default:
					predicate = PredicateCallsUnresolved
					target = exprString(fset, fun)
					meta = `{"reason":"unsupported_call_expr"}`
				}

				if strings.TrimSpace(target) == "" {
					return true
				}

				allFacts = append(allFacts, Fact{
					Subject:    caller,
					Predicate:  predicate,
					Object:     target,
					Source:     "ast",
					FilePath:   rel,
					CommitHash: c.commitHash,
					Confidence: 1,
					MetaJSON:   meta,
				})
				return true
			})
		}
	}

	return dedupeFacts(allFacts), nil
}

func (c *CodeIndexer) resolvePackageID(dir string) (string, error) {
	cmd := exec.Command("go", "list", "-f", "{{.ImportPath}}", dir)
	cmd.Dir = c.repoRoot
	out, err := cmd.Output()
	if err == nil {
		return strings.TrimSpace(string(out)), nil
	}
	rel := c.toRepoRelative(dir)
	modulePath, modErr := modulePathFromGoMod(filepath.Join(c.repoRoot, "go.mod"))
	if modErr != nil {
		return "", fmt.Errorf("resolve package id for %s: %w", dir, err)
	}
	if rel == "." {
		return modulePath, nil
	}
	return modulePath + "/" + filepath.ToSlash(rel), nil
}

func modulePathFromGoMod(path string) (string, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return "", err
	}
	for _, line := range strings.Split(string(data), "\n") {
		line = strings.TrimSpace(line)
		if strings.HasPrefix(line, "module ") {
			return strings.TrimSpace(strings.TrimPrefix(line, "module ")), nil
		}
	}
	return "", fmt.Errorf("module path not found in go.mod")
}

func collectImports(file *ast.File) map[string]string {
	out := map[string]string{}
	for _, imp := range file.Imports {
		raw := strings.Trim(imp.Path.Value, `"`)
		if raw == "" {
			continue
		}
		if imp.Name != nil && imp.Name.Name != "" && imp.Name.Name != "_" && imp.Name.Name != "." {
			out[imp.Name.Name] = raw
			continue
		}
		parts := strings.Split(raw, "/")
		out[parts[len(parts)-1]] = raw
	}
	return out
}

func qualifiedFunctionName(fn *ast.FuncDecl) string {
	base := fn.Name.Name
	if fn.Recv == nil || len(fn.Recv.List) == 0 {
		return base
	}
	receiver := receiverTypeName(fn.Recv.List[0].Type)
	if receiver == "" {
		return base
	}
	return receiver + "." + base
}

func receiverTypeName(expr ast.Expr) string {
	switch t := expr.(type) {
	case *ast.Ident:
		return t.Name
	case *ast.StarExpr:
		return receiverTypeName(t.X)
	case *ast.IndexExpr:
		return receiverTypeName(t.X)
	case *ast.IndexListExpr:
		return receiverTypeName(t.X)
	case *ast.SelectorExpr:
		if id, ok := t.X.(*ast.Ident); ok {
			return id.Name + "." + t.Sel.Name
		}
		return t.Sel.Name
	default:
		return ""
	}
}

func exprString(fset *token.FileSet, n ast.Node) string {
	var buf bytes.Buffer
	if err := format.Node(&buf, fset, n); err != nil {
		return ""
	}
	return strings.TrimSpace(buf.String())
}

func dedupeFacts(in []Fact) []Fact {
	seen := map[string]struct{}{}
	out := make([]Fact, 0, len(in))
	for _, f := range in {
		key := strings.Join([]string{
			f.Subject, f.Predicate, f.Object, f.FilePath, f.CommitHash, f.MetaJSON,
		}, "\x00")
		if _, ok := seen[key]; ok {
			continue
		}
		seen[key] = struct{}{}
		out = append(out, f)
	}
	sort.Slice(out, func(i, j int) bool {
		if out[i].Subject != out[j].Subject {
			return out[i].Subject < out[j].Subject
		}
		if out[i].Predicate != out[j].Predicate {
			return out[i].Predicate < out[j].Predicate
		}
		if out[i].Object != out[j].Object {
			return out[i].Object < out[j].Object
		}
		return out[i].FilePath < out[j].FilePath
	})
	return out
}

func (c *CodeIndexer) toRepoRelative(absPath string) string {
	rel, err := filepath.Rel(c.repoRoot, absPath)
	if err != nil {
		return filepath.ToSlash(absPath)
	}
	return filepath.ToSlash(rel)
}

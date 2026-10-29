package security_test

import (
	"go/ast"
	"go/parser"
	"go/token"
	"io/fs"
	"path/filepath"
	"runtime"
	"strings"
	"testing"
)

// root is the backend folder, found from this file so the tests run from anywhere.
func root(t testing.TB) string {
	t.Helper()
	_, file, _, ok := runtime.Caller(0)
	if !ok {
		t.Fatal("cannot find the test file")
	}
	return filepath.Clean(filepath.Join(filepath.Dir(file), "..", ".."))
}

// source is one parsed production file (no tests, no generated code).
type source struct {
	rel  string
	dir  string
	file *ast.File
	fset *token.FileSet
}

// production parses every non-test Go file under the given folders of the backend.
func production(t testing.TB, folders ...string) []source {
	t.Helper()
	var out []source
	for _, folder := range folders {
		base := filepath.Join(root(t), folder)
		err := filepath.WalkDir(base, func(path string, d fs.DirEntry, err error) error {
			if err != nil || d.IsDir() || !strings.HasSuffix(path, ".go") || strings.HasSuffix(path, "_test.go") || strings.HasSuffix(path, "_gen.go") {
				return err
			}
			fset := token.NewFileSet()
			f, err := parser.ParseFile(fset, path, nil, parser.SkipObjectResolution)
			if err != nil {
				return err
			}
			rel, _ := filepath.Rel(root(t), path)
			out = append(out, source{rel: filepath.ToSlash(rel), dir: filepath.ToSlash(filepath.Dir(rel)), file: f, fset: fset})
			return nil
		})
		if err != nil {
			t.Fatal(err)
		}
	}
	if len(out) == 0 {
		t.Fatal("no source files found")
	}
	return out
}

// constNames are the constants declared at the top of a file's package folder.
func constNames(files []source) map[string]map[string]bool {
	out := map[string]map[string]bool{}
	for _, s := range files {
		if out[s.dir] == nil {
			out[s.dir] = map[string]bool{}
		}
		for _, d := range s.file.Decls {
			if g, ok := d.(*ast.GenDecl); ok && g.Tok == token.CONST {
				for _, sp := range g.Specs {
					for _, n := range sp.(*ast.ValueSpec).Names {
						out[s.dir][n.Name] = true
					}
				}
			}
		}
	}
	return out
}

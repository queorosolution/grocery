package security_test

import (
	"fmt"
	"go/ast"
	"go/parser"
	"go/token"
	"os"
	"path/filepath"
	"regexp"
	"testing"
)

// fixedText reports whether e is text the programmer wrote: a literal, a
// constant, a local made only of such text, such pieces joined with +, or a
// Sprintf of a literal whose only verbs are %d (a number cannot carry SQL).
func fixedText(e ast.Expr, consts, locals map[string]bool) bool {
	switch v := e.(type) {
	case *ast.BasicLit:
		return v.Kind == token.STRING
	case *ast.Ident:
		return consts[v.Name] || locals[v.Name]
	case *ast.ParenExpr:
		return fixedText(v.X, consts, locals)
	case *ast.BinaryExpr:
		return v.Op == token.ADD && fixedText(v.X, consts, locals) && fixedText(v.Y, consts, locals)
	case *ast.CallExpr:
		return numbersOnly(v)
	}
	return false
}

// numbersOnly is fmt.Sprintf("literal with %d", ...).
func numbersOnly(c *ast.CallExpr) bool {
	sel, ok := c.Fun.(*ast.SelectorExpr)
	if !ok || sel.Sel.Name != "Sprintf" || len(c.Args) == 0 {
		return false
	}
	lit, ok := c.Args[0].(*ast.BasicLit)
	if !ok || lit.Kind != token.STRING {
		return false
	}
	verbs := regexp.MustCompile(`%[^%]`).FindAllString(lit.Value, -1)
	for _, v := range verbs {
		if v != "%d" {
			return false
		}
	}
	return len(verbs) > 0
}

// fixedLocals are the local variables of a function that only ever hold fixed
// text: q := `SELECT ...`, then q += ` FOR UPDATE`.
func fixedLocals(fn *ast.FuncDecl, consts map[string]bool) map[string]bool {
	bad := map[string]bool{}
	seen := map[string]bool{}
	if fn.Body == nil {
		return seen
	}
	// Repeat so a local built from another local settles.
	for range 3 {
		ast.Inspect(fn.Body, func(n ast.Node) bool {
			as, ok := n.(*ast.AssignStmt)
			if !ok || len(as.Lhs) != len(as.Rhs) {
				return true
			}
			for i, l := range as.Lhs {
				id, ok := l.(*ast.Ident)
				if !ok {
					continue
				}
				if as.Tok != token.DEFINE && as.Tok != token.ASSIGN && as.Tok != token.ADD_ASSIGN {
					continue
				}
				if !fixedText(as.Rhs[i], consts, seen) {
					bad[id.Name] = true
					continue
				}
				seen[id.Name] = true
			}
			return true
		})
	}
	for name := range bad {
		delete(seen, name)
	}
	return seen
}

var sqlMethods = map[string]bool{"Query": true, "QueryRow": true, "Exec": true, "QueryContext": true, "ExecContext": true, "QueryRowContext": true}

// builtSQL are the two places where SQL text is not fixed, and why that is safe.
var builtSQL = map[string]string{
	"internal/shared/db/tx.go": "the wrapper passes on the text its callers give; the callers are checked here",
	"internal/connector/source/postgres.go": "the shop's table and column names are checked against the database's own list and " +
		"quoted with pgx.Identifier (SQLI-02 tests); every value is a parameter",
}

// unsafeSQL returns the calls in the file whose SQL text is not fixed, as
// "file:line: method", and how many queries it looked at.
func unsafeSQL(s source, consts map[string]bool) (bad []string, checked int) {
	for _, d := range s.file.Decls {
		fn, ok := d.(*ast.FuncDecl)
		if !ok || fn.Body == nil {
			continue
		}
		locals := fixedLocals(fn, consts)
		ast.Inspect(fn.Body, func(n ast.Node) bool {
			call, ok := n.(*ast.CallExpr)
			if !ok {
				return true
			}
			sel, ok := call.Fun.(*ast.SelectorExpr)
			if !ok || !sqlMethods[sel.Sel.Name] || len(call.Args) < 2 || !isContext(call.Args[0]) {
				return true
			}
			checked++
			if !fixedText(call.Args[1], consts, locals) {
				bad = append(bad, fmt.Sprintf("%s:%d: %s", s.rel, s.fset.Position(call.Pos()).Line, sel.Sel.Name))
			}
			return true
		})
	}
	return bad, checked
}

// SQLI-01: the text of every query is written in the code. A value can only
// reach the database as a parameter, so no string is ever built from input.
func TestSQL_TextIsAlwaysFixed(t *testing.T) {
	files := production(t, "internal", "cmd")
	consts := constNames(files)
	total := 0
	for _, s := range files {
		if _, ok := builtSQL[s.rel]; ok {
			continue
		}
		bad, n := unsafeSQL(s, consts[s.dir])
		total += n
		for _, b := range bad {
			t.Errorf("%s: the SQL text is not fixed text; use a parameter", b)
		}
	}
	if total < 200 {
		t.Fatalf("only %d queries were checked; the scan lost its way", total)
	}
	for rel := range builtSQL {
		if _, err := os.Stat(filepath.Join(root(t), rel)); err != nil {
			t.Errorf("%s is listed as built SQL but is gone: %v", rel, err)
		}
	}
}

// The scan must be able to fail: these are the ways SQL gets built from input.
func TestSQL_ScanFindsBuiltText(t *testing.T) {
	src := `package x
const base = "SELECT 1 "
func fine(ctx context.Context, tx T, id string) {
	q := base + "FROM a WHERE id = $1"
	q += " FOR UPDATE"
	tx.QueryRow(ctx, q, id)
	tx.Exec(ctx, fmt.Sprintf("SET LOCAL x = %d", 5))
	tx.Exec(ctx, "DELETE FROM a WHERE id = $1", id)
}
func bad(ctx context.Context, tx T, name string) {
	tx.Query(ctx, "SELECT * FROM "+name)
	tx.Exec(ctx, fmt.Sprintf("DELETE FROM a WHERE n = '%s'", name))
	q := "SELECT 1 WHERE n = '" + name + "'"
	tx.QueryRow(ctx, q)
	r := other.Query("not sql")
	_ = r
}
`
	fset := token.NewFileSet()
	f, err := parser.ParseFile(fset, "x.go", src, parser.SkipObjectResolution)
	if err != nil {
		t.Fatal(err)
	}
	bad, checked := unsafeSQL(source{rel: "x.go", file: f, fset: fset}, map[string]bool{"base": true})
	if checked != 6 || len(bad) != 3 {
		t.Fatalf("checked %d, bad %v", checked, bad)
	}
}

// isContext tells (ctx, sql, ...) calls of pgx from other methods called Query or Exec.
func isContext(e ast.Expr) bool {
	switch v := e.(type) {
	case *ast.Ident:
		return v.Name == "ctx" || v.Name == "c"
	case *ast.CallExpr:
		sel, ok := v.Fun.(*ast.SelectorExpr)
		return ok && sel.Sel.Name == "Context"
	}
	return false
}

// outbound are the packages that make HTTP requests, and where each URL comes from.
// A new one must be added here by someone who has checked that the address is
// ours (configuration or the secret store) and never something a user typed.
var outbound = map[string]string{
	"internal/shared/turnstile": "Cloudflare's verify URL, fixed in code",
	"internal/cartclient":       "CART_API_BASE from configuration",
	"internal/notify/sms":       "SMS_API_URL from configuration, https only, no redirects",
	"internal/site":             "the Pages deploy hook from the secret store",
	"cmd/fakepayhere":           "a developer tool that posts to the local API",
}

var clientCalls = map[string]bool{"Get": true, "Head": true, "Post": true, "PostForm": true, "NewRequest": true, "NewRequestWithContext": true}

// usesHTTPClient reports whether the file makes HTTP requests.
func usesHTTPClient(f *ast.File) bool {
	found := false
	ast.Inspect(f, func(n ast.Node) bool {
		switch v := n.(type) {
		case *ast.SelectorExpr:
			if x, ok := v.X.(*ast.Ident); ok && x.Name == "http" && (clientCalls[v.Sel.Name] || v.Sel.Name == "DefaultClient" || v.Sel.Name == "DefaultTransport") {
				found = true
			}
		case *ast.CompositeLit:
			if sel, ok := v.Type.(*ast.SelectorExpr); ok {
				if x, ok := sel.X.(*ast.Ident); ok && x.Name == "http" && (sel.Sel.Name == "Client" || sel.Sel.Name == "Transport") {
					found = true
				}
			}
		}
		return !found
	})
	return found
}

// SSRF-03: the server never fetches an address a user gave. Only the listed
// packages make HTTP requests, and each takes its address from our own setup.
func TestSSRF_OnlyKnownPackagesCallOut(t *testing.T) {
	seen := map[string]bool{}
	for _, s := range production(t, "internal", "cmd") {
		if usesHTTPClient(s.file) {
			seen[s.dir] = true
		}
	}
	for dir := range seen {
		if _, ok := outbound[dir]; !ok {
			t.Errorf("%s makes HTTP requests but is not in the list of outbound packages (SSRF-03): check where its address comes from, then list it", dir)
		}
	}
	for dir, why := range outbound {
		if !seen[dir] {
			t.Errorf("%s is listed as outbound (%s) but makes no request any more: remove it from the list", dir, why)
		}
	}
}

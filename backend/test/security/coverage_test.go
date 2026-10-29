package security_test

import (
	"io/fs"
	"os"
	"path/filepath"
	"regexp"
	"sort"
	"strings"
	"testing"
)

// docs/11 section 1 says every C and H check has an automated test. This test
// reads that table and holds the project to it: a row with no test named after
// it fails here, and so does a gap that was closed but is still listed.
//
// A test belongs to a check when its file names the check ID (for example
// "AUTHZ-01") in a comment or a test name. Web tests (web/**/*.test.ts, web/e2e)
// follow the same rule.

// openGaps are the C and H checks with no automated test yet. Each says what is
// missing and when it arrives. The list may only shrink.
var openGaps = map[string]string{
	"XSS-01":  "the lint rule react/no-danger is on and guarded below; the browser check with a seeded product is the E2E suite (P34)",
	"XSS-02":  "the /search page does not exist yet (P29); its test must render q as text",
	"DATA-06": "a restore drill on Neon, done by hand once before go-live and noted in docs/14 (P35); it cannot be a test",
}

var idPattern = regexp.MustCompile(`\b(?:XSS|CSRF|CORS|SSRF|SQLI|HDR|LOG|AUTHN|AUTHZ|BIZ|DATA|INFRA|SUP)-\d\d\b`)

// checkRows reads docs/11 and returns the C and H check IDs, sorted.
func checkRows(t testing.TB) []string {
	t.Helper()
	var ids []string
	for _, line := range strings.Split(repoFile(t, "docs/11-security-audit.md"), "\n") {
		cols := strings.Split(strings.Trim(strings.TrimSpace(line), "|"), "|")
		if len(cols) < 5 || !idPattern.MatchString(strings.TrimSpace(cols[0])) {
			continue
		}
		if sev := strings.TrimSpace(cols[len(cols)-2]); sev == "C" || sev == "H" {
			ids = append(ids, strings.TrimSpace(cols[0]))
		}
	}
	sort.Strings(ids)
	return ids
}

// testedIDs maps each check ID to the test files that name it.
func testedIDs(t testing.TB) map[string][]string {
	t.Helper()
	repo := filepath.Join(root(t), "..")
	out := map[string][]string{}
	self := "backend/test/security/coverage_test.go"
	err := filepath.WalkDir(repo, func(path string, d fs.DirEntry, err error) error {
		if err != nil {
			return err
		}
		rel, _ := filepath.Rel(repo, path)
		rel = filepath.ToSlash(rel)
		if d.IsDir() {
			if d.Name() == "node_modules" || d.Name() == ".git" || d.Name() == ".next" || d.Name() == "out" || d.Name() == ".aws-sam" {
				return fs.SkipDir
			}
			return nil
		}
		isTest := strings.HasSuffix(rel, "_test.go") || regexp.MustCompile(`\.(test|spec)\.tsx?$`).MatchString(rel)
		if !isTest || rel == self {
			return nil
		}
		raw, err := os.ReadFile(path)
		if err != nil {
			return err
		}
		seen := map[string]bool{}
		for _, id := range idPattern.FindAllString(string(raw), -1) {
			if !seen[id] {
				seen[id] = true
				out[id] = append(out[id], rel)
			}
		}
		return nil
	})
	if err != nil {
		t.Fatal(err)
	}
	return out
}

func TestCoverage_EveryCriticalAndHighCheckHasATest(t *testing.T) {
	rows := checkRows(t)
	if len(rows) < 50 {
		t.Fatalf("read %d C and H rows from docs/11; the table changed shape", len(rows))
	}
	tested := testedIDs(t)
	inTable := map[string]bool{}
	for _, id := range rows {
		inTable[id] = true
		_, gap := openGaps[id]
		switch {
		case len(tested[id]) == 0 && !gap:
			t.Errorf("%s (C or H) has no test: name the ID in the test that checks it, or list it in openGaps with the reason", id)
		case len(tested[id]) > 0 && gap:
			t.Errorf("%s now has a test (%s): remove it from openGaps", id, tested[id][0])
		}
	}
	for id, why := range openGaps {
		if !inTable[id] {
			t.Errorf("openGaps lists %s, which is not a C or H row of docs/11", id)
		}
		if strings.TrimSpace(why) == "" {
			t.Errorf("openGaps %s has no reason", id)
		}
	}
	t.Logf("%d C and H checks: %d tested, %d open", len(rows), len(rows)-len(openGaps), len(openGaps))
}

// docs/11 section 2.1 lists where each C and H check is tested; it must name every one.
func TestCoverage_DocsListEveryCheck(t *testing.T) {
	doc := repoFile(t, "docs/11-security-audit.md")
	i := strings.Index(doc, "### 2.1")
	if i < 0 {
		t.Fatal("docs/11 has no section 2.1")
	}
	section := doc[i:]
	if j := strings.Index(section, "\n## "); j > 0 {
		section = section[:j]
	}
	for _, id := range checkRows(t) {
		if !strings.Contains(section, id) {
			t.Errorf("docs/11 section 2.1 does not list %s", id)
		}
	}
}

// The scan must be able to fail: this is a check that no test names.
func TestCoverage_ScanSeesTheTests(t *testing.T) {
	tested := testedIDs(t)
	if len(tested["AUTHZ-02"]) == 0 || len(tested["BIZ-04"]) == 0 || len(tested["SQLI-01"]) == 0 {
		t.Fatalf("the scan does not see the tests that are known to exist: %v", tested["AUTHZ-02"])
	}
	if len(tested["NOPE-99"]) != 0 {
		t.Fatal("an ID nobody wrote is reported as tested")
	}
}

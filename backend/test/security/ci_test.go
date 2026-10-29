package security_test

import (
	"regexp"
	"strings"
	"testing"
)

// Some checks of docs/11 run in CI and not in go test (a scanner, an audit, the
// schema tests). These tests keep that promise: if a job or a step is taken out
// of the workflow, or a secret file is no longer ignored, a test fails.

func repoFile(t testing.TB, rel string) string { return readFile(t, "../"+rel) }

func TestCI_RunsTheChecksOfDocs11(t *testing.T) {
	ci := repoFile(t, ".github/workflows/ci.yml")
	for id, step := range map[string]string{
		"SQLI-01 / gosec":          "gosec ",
		"static analysis":          "staticcheck ./...",
		"SUP-01":                   "govulncheck ./...",
		"races (BIZ-03, BIZ-04)":   "go test -race ./...",
		"AUTHZ-09 (RLS, 30 tests)": "make test-schema",
		"contracts":                "make check-contracts",
		"DATA-02":                  "gitleaks detect",
		"SUP-02":                   "npm audit --audit-level=high",
		"INFRA-01 (cfn-lint)":      "make validate",
		"XSS-08 (CSP hashes)":      "npm run check:csp",
	} {
		if !strings.Contains(ci, step) {
			t.Errorf("%s: the CI workflow no longer runs %q", id, step)
		}
	}
	if !strings.Contains(ci, `REQUIRE_TEST_DB: "1"`) {
		t.Error("CI must fail, not skip, when the test database is missing")
	}
}

// DATA-02: the secret scanner has a config, and the usual secret files are not tracked.
func TestSecrets_ScannerAndIgnoreFile(t *testing.T) {
	if !strings.Contains(repoFile(t, ".gitleaks.toml"), "useDefault = true") {
		t.Error("gitleaks config no longer extends the default rules")
	}
	ignore := repoFile(t, ".gitignore")
	for _, want := range []string{".env", ".env.*", "!.env.example"} {
		if !strings.Contains(ignore, want) {
			t.Errorf(".gitignore does not hold %q", want)
		}
	}
}

// SUP-03: every action is pinned to a commit, and the token can only read.
func TestCI_ActionsArePinned(t *testing.T) {
	pinned := regexp.MustCompile(`uses:\s+\S+@[0-9a-f]{40}\b`)
	for _, f := range []string{".github/workflows/ci.yml", ".github/workflows/deploy.yml"} {
		src := repoFile(t, f)
		for _, line := range strings.Split(src, "\n") {
			if strings.Contains(line, "uses:") && !strings.Contains(line, "./") && !pinned.MatchString(line) {
				t.Errorf("%s: action not pinned to a commit: %s", f, strings.TrimSpace(line))
			}
		}
		if !strings.Contains(src, "permissions:") {
			t.Errorf("%s has no permissions block", f)
		}
	}
}

// XSS-08, XSS-09 and the lint rules against script injection are web controls that run in CI.
// This keeps them wired in until a browser test replaces the need.
func TestWeb_SecurityControlsAreWired(t *testing.T) {
	lint := repoFile(t, "web/eslint.config.mjs")
	for _, rule := range []string{`"react/no-danger": "error"`, `"no-eval": "error"`, `"no-implied-eval": "error"`} {
		if !strings.Contains(lint, rule) {
			t.Errorf("web lint rules: %s is not set", rule)
		}
	}
	pkg := repoFile(t, "web/package.json")
	for check, want := range map[string]string{
		"XSS-09 runs with the lint": `"lint": "eslint . && npm run check:env"`,
		"XSS-08 runs after build":   `node scripts/csp-hashes.mjs"`,
		"XSS-08 has a check mode":   `"check:csp": "node scripts/csp-hashes.mjs --check"`,
	} {
		if !strings.Contains(pkg, want) {
			t.Errorf("web package.json: %s (%s)", check, want)
		}
	}
}

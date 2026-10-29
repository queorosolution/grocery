package security_test

import (
	"os"
	"path/filepath"
	"regexp"
	"strings"
	"testing"
)

func readFile(t testing.TB, rel string) string {
	t.Helper()
	raw, err := os.ReadFile(filepath.Join(root(t), rel))
	if err != nil {
		t.Fatal(err)
	}
	return string(raw)
}

// indent is the number of leading spaces of a line.
func indent(line string) int { return len(line) - len(strings.TrimLeft(line, " ")) }

// valuesOf returns every value under a key of the template, inline ([a, b]),
// single, or as a "- item" list on the lines below.
func valuesOf(template, key string) []string {
	var out []string
	lines := strings.Split(template, "\n")
	for i, line := range lines {
		trim := strings.TrimPrefix(strings.TrimSpace(line), "- ")
		if !strings.HasPrefix(trim, key+":") {
			continue
		}
		rest := strings.TrimSpace(strings.TrimPrefix(trim, key+":"))
		if rest != "" {
			for _, v := range strings.Split(strings.Trim(rest, "[]"), ",") {
				out = append(out, strings.Trim(strings.TrimSpace(v), `"'`))
			}
			continue
		}
		for _, next := range lines[i+1:] {
			nt := strings.TrimSpace(next)
			if !strings.HasPrefix(nt, "- ") || strings.Contains(nt, ": ") {
				break
			}
			out = append(out, strings.Trim(strings.TrimSpace(strings.TrimPrefix(nt, "- ")), `"'`))
		}
	}
	return out
}

// INFRA-01: no function and no role may do everything. Every action is named
// (a trust policy's sts:AssumeRole is the one policy that names a service).
func TestTemplate_NoWildcardPermissions(t *testing.T) {
	tpl := readFile(t, "template.yaml")
	actions := valuesOf(tpl, "Action")
	if len(actions) < 10 {
		t.Fatalf("found %d IAM actions; the scan lost its way", len(actions))
	}
	for _, a := range actions {
		if strings.Contains(a, "*") {
			t.Errorf("IAM action %q has a wildcard", a)
		}
	}
	for _, r := range valuesOf(tpl, "Resource") {
		if r == "*" {
			t.Error("a policy allows every resource")
		}
	}
	if regexp.MustCompile(`(?i)ManagedPolicyArns|AdministratorAccess|PowerUserAccess`).MatchString(tpl) {
		t.Error("the template attaches a managed policy; list the actions instead")
	}
}

func TestTemplate_ScanFindsWildcards(t *testing.T) {
	bad := "Policies:\n  - Statement:\n      - Action: 'ssm:*'\n        Resource: '*'\n      - Action:\n          - s3:GetObject\n          - \"*\"\n        Resource:\n          - '*'\n"
	if got := valuesOf(bad, "Action"); len(got) != 3 || got[0] != "ssm:*" || got[2] != "*" {
		t.Errorf("actions = %v", got)
	}
	if got := valuesOf(bad, "Resource"); len(got) != 2 || got[0] != "*" || got[1] != "*" {
		t.Errorf("resources = %v", got)
	}
}

var (
	secretName  = regexp.MustCompile(`(?i)(secret|token|password|passwd|pepper|private|api_?key|database_url|dsn|credential)`)
	secretValue = regexp.MustCompile(`^(sk_|pk_)|^[A-Za-z0-9+/=_-]{32,}$|postgres(ql)?://[^@\s]*:[^@\s]*@|AKIA[0-9A-Z]{16}|^Bearer `)
)

// envVariables are the "NAME: value" pairs under every "Variables:" key.
func envVariables(template string) map[string]string {
	out := map[string]string{}
	lines := strings.Split(template, "\n")
	for i, line := range lines {
		if strings.TrimSpace(line) != "Variables:" {
			continue
		}
		base := indent(line)
		for _, next := range lines[i+1:] {
			if strings.TrimSpace(next) == "" || strings.HasPrefix(strings.TrimSpace(next), "#") {
				continue
			}
			if indent(next) <= base {
				break
			}
			if k, v, ok := strings.Cut(strings.TrimSpace(next), ":"); ok {
				out[k] = strings.TrimSpace(v)
			}
		}
	}
	return out
}

// INFRA-06: the functions get the name of the SSM folder and plain settings,
// never a secret. No variable is called like a secret, and no value looks like one.
func TestTemplate_EnvHoldsNoSecret(t *testing.T) {
	tpl := readFile(t, "template.yaml")
	env := envVariables(tpl)
	if len(env) < 20 {
		t.Fatalf("found %d variables; the scan lost its way", len(env))
	}
	for k, v := range env {
		if secretName.MatchString(k) {
			t.Errorf("variable %s is named like a secret: secrets are read from SSM by name", k)
		}
		if lit := strings.Trim(v, `"'`); !strings.HasPrefix(v, "!") && secretValue.MatchString(lit) {
			t.Errorf("variable %s holds a value that looks like a key: %q", k, lit)
		}
	}
	// A secret must not come in as a stack parameter either: those show in the console.
	for _, line := range strings.Split(strings.SplitN(strings.SplitN(tpl, "\nParameters:", 2)[1], "\nConditions:", 2)[0], "\n") {
		if indent(line) == 2 {
			if name, _, _ := strings.Cut(strings.TrimSpace(line), ":"); secretName.MatchString(name) {
				t.Errorf("parameter %s is named like a secret", name)
			}
		}
	}
}

// fakeKey looks like a payment key but is built here, so no scanner takes it for a real one.
var fakeKey = "sk" + "_" + "live" + "_" + strings.Repeat("a1b2", 8)

func TestTemplate_EnvScanFindsSecrets(t *testing.T) {
	tpl := "Globals:\n  Function:\n    Environment:\n      Variables:\n        # a comment\n        SSM_PREFIX: !Sub \"/gp/${Env}\"\n        PAYHERE_SECRET: abc\n        OTHER: " + fakeKey + "\n        DB: postgres://u:pw@host/db\n"
	env := envVariables(tpl)
	if len(env) != 4 || env["PAYHERE_SECRET"] != "abc" {
		t.Fatalf("env = %v", env)
	}
	if !secretName.MatchString("PAYHERE_SECRET") || !secretValue.MatchString(env["OTHER"]) || !secretValue.MatchString(env["DB"]) || secretValue.MatchString("info") {
		t.Error("the patterns miss a secret or flag a plain value")
	}
}

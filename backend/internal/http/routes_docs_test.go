package http_test

import (
	"os"
	"regexp"
	"strconv"
	"strings"
	"testing"

	apihttp "gp/internal/http"
	"gp/test/apptest"
)

var (
	// `GET/POST /x`, and `POST /x/activate` · `/pause` (the second path sits beside the first).
	routeToken   = regexp.MustCompile("`((?:GET|POST|PUT|PATCH|DELETE)(?:/(?:GET|POST|PUT|PATCH|DELETE))*) (/[^`\\s]*)`(?: · `/([a-z-]+)`)?")
	routeRow     = regexp.MustCompile("(?m)^\\| `(GET|POST|PUT|PATCH|DELETE)` \\| `(/[^`]*)`")
	pathParam    = regexp.MustCompile(`\{[^}]*\}`)
	sectionStart = regexp.MustCompile(`^(\d+)\.`)
)

// docsRoutes reads the routes of docs/06 as "METHOD /path" with every {name}
// written as {}. Headings and tables name them; sections 8, 9 and 10 leave out
// their prefix.
func docsRoutes(doc string) map[string]bool {
	out := map[string]bool{}
	for _, section := range strings.Split(doc, "\n## ")[1:] {
		m := sectionStart.FindStringSubmatch(section)
		if m == nil {
			continue
		}
		n, _ := strconv.Atoi(m[1])
		prefix := map[int]string{8: "/console/shops/{shopId}", 9: "/driver", 10: "/admin"}[n]
		pairs := append(routeToken.FindAllStringSubmatch(section, -1), routeRow.FindAllStringSubmatch(section, -1)...)
		for _, p := range pairs {
			path, _, _ := strings.Cut(p[2], "?")
			paths := []string{path}
			if len(p) > 3 && p[3] != "" {
				paths = append(paths, path[:strings.LastIndex(path, "/")+1]+p[3])
			}
			for _, path := range paths {
				if prefix != "" && !strings.HasPrefix(path, prefix+"/") && !strings.HasPrefix(path, "/admin/") && !strings.HasPrefix(path, "/console/") {
					path = prefix + path
				}
				for _, method := range strings.Split(p[1], "/") {
					out[method+" "+pathParam.ReplaceAllString(path, "{}")] = true
				}
			}
		}
	}
	return out
}

func codeRoutes(t *testing.T) map[string]bool {
	t.Helper()
	out := map[string]bool{}
	for _, rt := range apihttp.Routes(apptest.New(t).Deps) {
		out[rt.Method+" "+pathParam.ReplaceAllString(rt.Pattern, "{}")] = true
	}
	return out
}

// notBuiltYet are routes the docs describe that come with a later prompt.
var notBuiltYet = map[string]string{
	"DELETE /me": "FR-AUTH-09 / DATA-07 (account deletion): in no prompt yet and its rules for open orders and shop members are not in the docs; see the P27 pull request",
}

// docs/15: every route in docs/06 is in routes.go (TestEveryRouteHasValidGuard
// checks the guard), and every route in routes.go is in docs/06.
func TestRoutes_MatchDocs(t *testing.T) {
	raw, err := os.ReadFile("../../../docs/06-api.md")
	if err != nil {
		t.Fatal(err)
	}
	documented, built := docsRoutes(string(raw)), codeRoutes(t)
	if len(documented) < 100 {
		t.Fatalf("read %d routes from docs/06; the document changed shape", len(documented))
	}
	for r := range documented {
		if _, later := notBuiltYet[r]; !built[r] && !later {
			t.Errorf("docs/06 describes %s, but routes.go has no such route", r)
		}
	}
	for r, why := range notBuiltYet {
		if built[r] {
			t.Errorf("%s is built now: remove it from notBuiltYet (%s)", r, why)
		}
	}
	for r := range built {
		if !documented[r] {
			t.Errorf("routes.go has %s, but docs/06 does not describe it", r)
		}
	}
}

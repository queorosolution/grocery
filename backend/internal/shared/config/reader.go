package config

import (
	"fmt"
	"net/mail"
	"net/url"
	"regexp"
	"strconv"
	"strings"
)

var (
	domainRe    = regexp.MustCompile(`^([a-z0-9]([a-z0-9-]*[a-z0-9])?\.)+[a-z]{2,}$`)
	phoneRe     = regexp.MustCompile(`^\+94\d{9}$`)
	ssmPrefixRe = regexp.MustCompile(`^(/[a-z0-9][a-z0-9_-]*)+$`)
)

// reader collects every problem so one start-up shows them all, not one per run.
type reader struct {
	env      map[string]string
	problems []string
}

func newReader(environ []string) *reader {
	r := &reader{env: make(map[string]string, len(environ))}
	for _, kv := range environ {
		if name, value, ok := strings.Cut(kv, "="); ok {
			r.env[name] = value
		}
	}
	return r
}

func (r *reader) get(name string) string { return strings.TrimSpace(r.env[name]) }

func (r *reader) fail(name, why string) {
	r.problems = append(r.problems, name+" "+why)
}

func (r *reader) err() error {
	if len(r.problems) == 0 {
		return nil
	}
	return fmt.Errorf("config: %d problem(s):\n  - %s", len(r.problems), strings.Join(r.problems, "\n  - "))
}

// value returns the trimmed value; ok is false when it is empty (and, if
// required, a problem is recorded). Present values are always validated.
func (r *reader) value(name string, required bool) (string, bool) {
	v := r.get(name)
	if v == "" {
		if required {
			r.fail(name, "is required")
		}
		return "", false
	}
	return v, true
}

func (r *reader) str(name string, required bool) string {
	v, _ := r.value(name, required)
	return v
}

func (r *reader) oneOf(name string, options ...Env) Env {
	v, ok := r.value(name, true)
	if !ok {
		return ""
	}
	for _, o := range options {
		if Env(v) == o {
			return o
		}
	}
	r.fail(name, "must be one of local, dev, prod")
	return ""
}

func (r *reader) domain(name string) string {
	v, ok := r.value(name, true)
	if !ok {
		return ""
	}
	v = strings.ToLower(v)
	if !domainRe.MatchString(v) {
		r.fail(name, "must be a bare domain like example.com")
		return ""
	}
	return v
}

func (r *reader) phone(name string, required bool) string {
	v, ok := r.value(name, required)
	if ok && !phoneRe.MatchString(v) {
		r.fail(name, "must look like +94770000000")
		return ""
	}
	return v
}

func (r *reader) ssmPrefix(name string) string {
	v, ok := r.value(name, true)
	if ok && !ssmPrefixRe.MatchString(v) {
		r.fail(name, "must look like /gp/dev (lower case, no trailing slash)")
		return ""
	}
	return v
}

func (r *reader) positiveInt(name string) int {
	v, ok := r.value(name, true)
	if !ok {
		return 0
	}
	n, err := strconv.Atoi(v)
	if err != nil || n <= 0 {
		r.fail(name, "must be a positive whole number")
		return 0
	}
	return n
}

func (r *reader) boolean(name string) bool {
	switch v, _ := r.value(name, true); v {
	case "true":
		return true
	case "false", "":
		return false
	default:
		r.fail(name, "must be true or false")
		return false
	}
}

func (r *reader) emails(name string) []string {
	v, ok := r.value(name, true)
	if !ok {
		return nil
	}
	var out []string
	for _, item := range strings.Split(v, ",") {
		item = strings.ToLower(strings.TrimSpace(item))
		if addr, err := mail.ParseAddress(item); err != nil || addr.Address != item {
			r.fail(name, "must be a comma list of bare e-mail addresses")
			return nil
		}
		out = append(out, item)
	}
	return out
}

// urlRule says what a URL setting may look like.
type urlRule struct {
	required  bool
	allowHTTP bool // plain http, for local runs only
	allowPath bool // false for an origin such as https://www.example.com
}

func (r *reader) url(name string, rule urlRule) string {
	v, ok := r.value(name, rule.required)
	if !ok {
		return ""
	}
	if why := checkURL(v, rule); why != "" {
		r.fail(name, why)
		return ""
	}
	return v
}

func (r *reader) urls(name string, rule urlRule) []string {
	v, ok := r.value(name, rule.required)
	if !ok {
		return nil
	}
	var out []string
	for _, item := range strings.Split(v, ",") {
		item = strings.TrimSpace(item)
		if why := checkURL(item, rule); why != "" {
			r.fail(name, why)
			return nil
		}
		out = append(out, item)
	}
	return out
}

func checkURL(v string, rule urlRule) string {
	u, err := url.Parse(v)
	switch {
	case err != nil || u.Host == "" || u.User != nil || u.RawQuery != "" || u.Fragment != "":
		return "must be a URL without credentials, query or fragment"
	case u.Scheme != "https" && !(rule.allowHTTP && u.Scheme == "http"):
		return "must use https"
	case strings.Contains(v, "*"):
		return "must not contain a wildcard"
	case strings.HasSuffix(v, "/"):
		return "must not end with a slash"
	case !rule.allowPath && u.Path != "":
		return "must be an origin like https://example.com, without a path"
	}
	return ""
}

// refuseLocalOnly stops a dev or prod start-up that carries local-only
// settings, which would send secrets through plain environment variables.
func (r *reader) refuseLocalOnly(names []string) {
	for _, name := range names {
		if _, set := r.env[name]; set {
			r.fail(name, "is local-only: remove it unless APP_ENV is local")
		}
	}
	for name := range r.env {
		if strings.HasPrefix(name, "SECRET_") {
			r.fail(name, "is local-only: secrets come from SSM unless APP_ENV is local")
		}
	}
}

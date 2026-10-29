package source

import (
	"errors"
	"net/url"
	"regexp"
	"strconv"
	"strings"
)

// maxConnStringLen keeps a pasted string small; real ones are about 150 characters.
const maxConnStringLen = 1000

// ErrBadConnString is returned for a string we will not use. It never contains
// the string, which holds a password.
var ErrBadConnString = errors.New("source: bad connection string")

// allowedParams are the only options a connection string may carry. Others can
// read files on the server (sslrootcert, passfile, service) or change the host
// after our checks (host, port), so they are refused instead of ignored.
var allowedParams = map[string]bool{"sslmode": true, "channel_binding": true, "connect_timeout": true, "application_name": true}

// Target is a parsed connection string.
type Target struct {
	User, Password, Host, Database string
	Port                           int
}

// ParseConnString accepts postgres://user:password@host[:port]/database[?sslmode=…].
// It checks the shape only: whether the host may be reached is decided later,
// by the policy, on every connection.
func ParseConnString(raw string) (Target, error) {
	raw = strings.TrimSpace(raw)
	if raw == "" || len(raw) > maxConnStringLen {
		return Target{}, ErrBadConnString
	}
	u, err := url.Parse(raw)
	if err != nil || (u.Scheme != "postgres" && u.Scheme != "postgresql") || u.Opaque != "" || u.Fragment != "" {
		return Target{}, ErrBadConnString
	}
	pw, hasPW := u.User.Password()
	t := Target{User: u.User.Username(), Password: pw, Host: u.Hostname(), Port: postgresPort, Database: strings.TrimPrefix(u.Path, "/")}
	if p := u.Port(); p != "" {
		n, err := strconv.Atoi(p)
		if err != nil || n < 1 || n > 65535 {
			return Target{}, ErrBadConnString
		}
		t.Port = n
	}
	if t.User == "" || !hasPW || pw == "" || t.Host == "" || t.Database == "" || strings.Contains(t.Database, "/") {
		return Target{}, ErrBadConnString
	}
	for k, vals := range u.Query() {
		if !allowedParams[k] || len(vals) != 1 {
			return Target{}, ErrBadConnString
		}
	}
	// TLS is always used. A string that asks for no encryption is a mistake worth saying so.
	switch u.Query().Get("sslmode") {
	case "", "require", "verify-ca", "verify-full":
	default:
		return Target{}, ErrBadConnString
	}
	return t, nil
}

// Display is the string with the password hidden, safe to show and to store:
// postgres://user:•••@host/database (the port is shown when it is not 5432).
func (t Target) Display() string {
	host := t.Host
	if strings.Contains(host, ":") { // an IPv6 address
		host = "[" + host + "]"
	}
	if t.Port != postgresPort {
		host += ":" + strconv.Itoa(t.Port)
	}
	return "postgres://" + t.User + ":•••@" + host + "/" + t.Database
}

// URL rebuilds a clean connection URL for the driver from the parts, with the
// address to dial and nothing else from the original string.
func (t Target) url(addr string) string {
	u := url.URL{Scheme: "postgres", User: url.UserPassword(t.User, t.Password), Host: hostPort(addr, t.Port), Path: "/" + t.Database}
	return u.String()
}

func hostPort(host string, port int) string {
	if strings.Contains(host, ":") {
		host = "[" + host + "]"
	}
	return host + ":" + strconv.Itoa(port)
}

// Names are checked twice: here by shape, and against information_schema. The
// shape matches the CHECK constraints of the connectors table.
var identRe = regexp.MustCompile(`^[a-z_][a-z0-9_]{0,62}$`)

// IsIdent reports whether s is a plain lower-case SQL name.
func IsIdent(s string) bool { return identRe.MatchString(s) }

// TableName is a table or view, with its schema.
type TableName struct {
	Schema string `json:"schema"`
	Name   string `json:"name"`
}

func (t TableName) String() string { return t.Schema + "." + t.Name }

// ParseTable reads "schema.name" or "name" (the public schema).
func ParseTable(s string) (TableName, bool) {
	schema, name, found := strings.Cut(s, ".")
	if !found {
		schema, name = "public", s
	}
	if !IsIdent(schema) || !IsIdent(name) {
		return TableName{}, false
	}
	return TableName{Schema: schema, Name: name}, true
}

package source_test

import (
	"context"
	"errors"
	"net/netip"
	"strings"
	"testing"

	"gp/internal/connector/source"
)

type fakeResolver map[string][]string

func (f fakeResolver) LookupNetIP(_ context.Context, _, host string) ([]netip.Addr, error) {
	list, ok := f[host]
	if !ok {
		return nil, errors.New("no such host")
	}
	var out []netip.Addr
	for _, s := range list {
		out = append(out, netip.MustParseAddr(s))
	}
	return out, nil
}

var resolver = fakeResolver{
	"localhost":        {"127.0.0.1", "::1"},
	"rebind.evil.test": {"127.0.0.1"},
	"mixed.evil.test":  {"93.184.216.34", "10.0.0.5"},
	"meta.evil.test":   {"169.254.169.254"},
	"db.example.test":  {"93.184.216.34"},
	"v6.example.test":  {"2606:2800:220:1:248:1893:25c8:1946", "93.184.216.34"},
}

// SSRF-01: every address that points inside is refused, however it is written.
func TestPolicy_RefusesInternalTargets(t *testing.T) {
	p := source.Policy{Resolver: resolver}
	for _, host := range []string{
		"localhost", "127.0.0.1", "[::1]", "::1", "169.254.169.254", "10.0.0.5", "172.16.0.9", "192.168.1.1",
		"100.64.0.1", "0.0.0.0", "::", "fe80::1", "fd00:ec2::254", "fc00::1", "::ffff:127.0.0.1", "::ffff:10.0.0.1",
		"2130706433", "0x7f000001", "127.1", "0177.0.0.1", "017700000001", "192.168.1",
		"rebind.evil.test", "mixed.evil.test", "meta.evil.test", "nxdomain.example.test", "", "db.example.test.",
		"64:ff9b::7f00:1", "2002:7f00:1::1", "224.0.0.1", "fe80::1%eth0",
	} {
		if _, err := p.Resolve(context.Background(), host, 5432); !errors.Is(err, source.ErrRefused) {
			t.Errorf("host %q was not refused (err = %v)", host, err)
		}
	}
}

// SSRF-02: only port 5432 is allowed, whatever the host.
func TestPolicy_OnlyPort5432(t *testing.T) {
	p := source.Policy{Resolver: resolver}
	for _, port := range []int{0, 22, 80, 443, 3306, 5433, 6379, 65535} {
		if _, err := p.Resolve(context.Background(), "db.example.test", port); !errors.Is(err, source.ErrRefused) {
			t.Errorf("port %d was not refused", port)
		}
	}
	if _, err := p.Resolve(context.Background(), "db.example.test", 5432); err != nil {
		t.Errorf("port 5432 refused: %v", err)
	}
}

func TestPolicy_AllowsPublicTargets(t *testing.T) {
	p := source.Policy{Resolver: resolver}
	tests := map[string]string{
		"db.example.test":      "93.184.216.34",
		"DB.Example.Test":      "93.184.216.34",
		"v6.example.test":      "93.184.216.34", // IPv4 preferred
		"93.184.216.34":        "93.184.216.34",
		"2606:2800:220:1::1":   "2606:2800:220:1::1",
		"[2606:2800:220:1::1]": "2606:2800:220:1::1",
	}
	for host, want := range tests {
		got, err := p.Resolve(context.Background(), host, 5432)
		if err != nil || got.String() != want {
			t.Errorf("Resolve(%q) = %v, %v; want %s", host, got, err, want)
		}
	}
}

func TestPolicy_ErrorSaysNothing(t *testing.T) {
	p := source.Policy{Resolver: resolver}
	_, err := p.Resolve(context.Background(), "10.0.0.5", 22)
	if err == nil || strings.Contains(err.Error(), "10.0.0.5") || strings.Contains(err.Error(), "22") {
		t.Errorf("error leaks the target: %v", err)
	}
}

func TestParseConnString(t *testing.T) {
	good := "postgresql://gp_reader:s3cret%2Fpw@ep-cool-123.ap-southeast-1.aws.neon.tech/shopdb?sslmode=require&channel_binding=require"
	tg, err := source.ParseConnString(good)
	if err != nil {
		t.Fatal(err)
	}
	if tg.User != "gp_reader" || tg.Password != "s3cret/pw" || tg.Host != "ep-cool-123.ap-southeast-1.aws.neon.tech" || tg.Port != 5432 || tg.Database != "shopdb" {
		t.Errorf("parsed = %+v", tg)
	}
	if d := tg.Display(); d != "postgres://gp_reader:•••@ep-cool-123.ap-southeast-1.aws.neon.tech/shopdb" || strings.Contains(d, "s3cret") {
		t.Errorf("display = %q", d)
	}
	other, _ := source.ParseConnString("postgres://u:p@db.example.test:6543/d")
	if other.Port != 6543 || other.Display() != "postgres://u:•••@db.example.test:6543/d" {
		t.Errorf("port display = %q", other.Display())
	}

	for name, raw := range map[string]string{
		"empty":           "",
		"mysql":           "mysql://u:p@h/d",
		"no password":     "postgres://u@h/d",
		"empty password":  "postgres://u:@h/d",
		"no user":         "postgres://:p@h/d",
		"no database":     "postgres://u:p@h/",
		"nested database": "postgres://u:p@h/a/b",
		"no host":         "postgres://u:p@/d",
		"bad port":        "postgres://u:p@h:99999/d",
		"key=value":       "host=h user=u password=p dbname=d",
		"host override":   "postgres://u:p@h/d?host=127.0.0.1",
		"port override":   "postgres://u:p@h/d?port=22",
		"ssl root file":   "postgres://u:p@h/d?sslrootcert=/etc/passwd",
		"passfile":        "postgres://u:p@h/d?passfile=/root/.pgpass",
		"service":         "postgres://u:p@h/d?service=x",
		"no encryption":   "postgres://u:p@h/d?sslmode=disable",
		"prefer":          "postgres://u:p@h/d?sslmode=prefer",
		"repeated param":  "postgres://u:p@h/d?sslmode=require&sslmode=require",
		"too long":        "postgres://u:p@h/" + strings.Repeat("d", 1100),
		"fragment":        "postgres://u:p@h/d#x",
	} {
		if _, err := source.ParseConnString(raw); !errors.Is(err, source.ErrBadConnString) {
			t.Errorf("%s: accepted %q", name, raw)
		}
	}
}

func TestParseTable(t *testing.T) {
	ok := map[string]source.TableName{
		"gp_products":        {Schema: "public", Name: "gp_products"},
		"public.gp_products": {Schema: "public", Name: "gp_products"},
		"shop.items_2":       {Schema: "shop", Name: "items_2"},
	}
	for in, want := range ok {
		if got, found := source.ParseTable(in); !found || got != want {
			t.Errorf("ParseTable(%q) = %v, %v", in, got, found)
		}
	}
	for _, in := range []string{"", "Products", "a.b.c", `x"; DROP TABLE y;--`, "1abc", "a b", ".x", "x.", strings.Repeat("a", 64)} {
		if _, found := source.ParseTable(in); found {
			t.Errorf("ParseTable(%q) accepted", in)
		}
	}
}

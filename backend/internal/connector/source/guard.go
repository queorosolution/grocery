// Package source reads a shop's own Postgres database for the connector. It is
// the only place that opens a connection to an address an admin typed in, so it
// is also where the server-side request forgery rules live (SSRF-01, SSRF-02):
// the host is resolved here, every address must be public, and the connection
// goes to the address that was checked, never to the name.
package source

import (
	"context"
	"errors"
	"net"
	"net/netip"
	"strings"
)

// ErrRefused is the one error the guard returns. It says nothing about why, so
// a caller cannot be used to scan the network from inside it.
var ErrRefused = errors.New("source: target refused")

// postgresPort is the only port a shop database may use (SSRF-02).
const postgresPort = 5432

// Resolver is the part of net.Resolver the guard needs, so tests can make a
// name resolve to any address.
type Resolver interface {
	LookupNetIP(ctx context.Context, network, host string) ([]netip.Addr, error)
}

// Policy decides which targets may be connected to.
type Policy struct {
	// Resolver looks names up; nil means the system resolver.
	Resolver Resolver
	// Port is the one allowed port; 0 means 5432.
	Port int
	// AllowLocal lets loopback and private addresses through. Tests only: the
	// shop database of a real shop is never on one of those.
	AllowLocal bool
}

func (p Policy) port() int {
	if p.Port == 0 {
		return postgresPort
	}
	return p.Port
}

// Resolve checks the target and returns the address to connect to. A host that
// is already an address is checked as it is; a name is resolved and ALL of its
// addresses must be allowed, so a name that also points inside cannot win a race.
func (p Policy) Resolve(ctx context.Context, host string, port int) (netip.Addr, error) {
	if port != p.port() {
		return netip.Addr{}, ErrRefused
	}
	host = strings.ToLower(strings.TrimSpace(strings.Trim(host, "[]")))
	if host == "" || len(host) > 253 {
		return netip.Addr{}, ErrRefused
	}
	if a, err := netip.ParseAddr(host); err == nil {
		if a.Zone() != "" || !p.allowed(a) {
			return netip.Addr{}, ErrRefused
		}
		return a.Unmap(), nil
	}
	// Forms such as 2130706433, 0x7f000001 or 127.1 are read as addresses by
	// some resolvers. No real host name ends in a number, so refuse them all.
	if numericLastLabel(host) {
		return netip.Addr{}, ErrRefused
	}
	r := p.Resolver
	if r == nil {
		r = net.DefaultResolver
	}
	addrs, err := r.LookupNetIP(ctx, "ip", host)
	if err != nil || len(addrs) == 0 {
		return netip.Addr{}, ErrRefused
	}
	var pick netip.Addr
	for _, a := range addrs {
		if a.Zone() != "" || !p.allowed(a) {
			return netip.Addr{}, ErrRefused
		}
		a = a.Unmap()
		if !pick.IsValid() || (a.Is4() && pick.Is6()) { // IPv4 first: it is the one that works from every network
			pick = a
		}
	}
	return pick, nil
}

func (p Policy) allowed(a netip.Addr) bool {
	a = a.Unmap()
	if !a.IsValid() || a.IsUnspecified() {
		return false
	}
	return p.AllowLocal || !blocked(a)
}

func numericLastLabel(host string) bool {
	last := host
	if i := strings.LastIndexByte(host, '.'); i >= 0 {
		last = host[i+1:]
	}
	if last == "" || strings.HasPrefix(last, "0x") {
		return true
	}
	for _, c := range last {
		if c < '0' || c > '9' {
			return false
		}
	}
	return true
}

// blockedRanges are the public-looking ranges that are still not the internet.
var blockedRanges = prefixes(
	"0.0.0.0/8", "100.64.0.0/10", "192.0.0.0/24", "192.0.2.0/24", "198.18.0.0/15", "198.51.100.0/24",
	"203.0.113.0/24", "240.0.0.0/4",
	"64:ff9b::/96", "64:ff9b:1::/48", "100::/64", "2001:db8::/32", "2002::/16",
)

func prefixes(list ...string) []netip.Prefix {
	out := make([]netip.Prefix, len(list))
	for i, s := range list {
		out[i] = netip.MustParsePrefix(s)
	}
	return out
}

// blocked is true for loopback, private, link-local (which holds the cloud
// metadata address 169.254.169.254), unique-local, multicast and the ranges above.
func blocked(a netip.Addr) bool {
	if a.IsLoopback() || a.IsPrivate() || a.IsLinkLocalUnicast() || a.IsLinkLocalMulticast() ||
		a.IsInterfaceLocalMulticast() || a.IsMulticast() {
		return true
	}
	for _, p := range blockedRanges {
		if p.Contains(a) {
			return true
		}
	}
	return false
}

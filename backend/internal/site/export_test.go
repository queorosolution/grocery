package site

import "gp/internal/shared/secrets"

// NewRebuilderForTest allows an http:// hook, so tests can use a local server.
func NewRebuilderForTest(sec secrets.Getter) *Rebuilder {
	r := NewRebuilder(sec)
	r.allowHTTP = true
	return r
}

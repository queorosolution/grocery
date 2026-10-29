package httpx

import (
	"crypto/sha256"
	"encoding/hex"
	"net/http"
	"strings"
)

// Revalidated is a result a handler wants sent with an ETag (docs/06 section 1):
// a list a screen asks for again and again. The tag is a hash of the body, so it
// changes exactly when the answer does, and a client that sends it back in
// If-None-Match gets 304 with no body when nothing changed.
type Revalidated struct{ V any }

// JSONWithETag is JSON for a GET that supports If-None-Match. A 304 carries the
// tag and no body; the status of every other answer is the one given.
func JSONWithETag(w http.ResponseWriter, r *http.Request, status int, v any) {
	body, ok := encode(w, v)
	if !ok {
		return
	}
	sum := sha256.Sum256(body)
	tag := `"` + hex.EncodeToString(sum[:16]) + `"`
	w.Header().Set("ETag", tag)
	if status == http.StatusOK && matches(r.Header.Get("If-None-Match"), tag) {
		w.WriteHeader(http.StatusNotModified)
		return
	}
	writeBody(w, status, body)
}

// matches reports whether the If-None-Match header names the tag. It follows the
// weak comparison of RFC 9110: a W/ prefix is ignored and "*" matches anything.
func matches(header, tag string) bool {
	for _, c := range strings.Split(header, ",") {
		c = strings.TrimPrefix(strings.TrimSpace(c), "W/")
		if c == "*" || c == tag {
			return true
		}
	}
	return false
}

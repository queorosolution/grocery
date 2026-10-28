package apptest

import (
	"net/http/httptest"
	"testing"
)

// AdminAPI calls the real API as the super admin who passed the authenticator
// check. Paths start after /v1.
type AdminAPI struct {
	*App
	Session Session
}

// AsAdmin signs in the super admin once and returns a client for admin routes.
func (a *App) AsAdmin() AdminAPI { return AdminAPI{App: a, Session: a.Admin()} }

func (c AdminAPI) Call(method, path, body string) *httptest.ResponseRecorder {
	return c.App.Call(method, "/v1"+path, body, c.Session.Cookies()...)
}

// OK calls and fails the test on any answer of 300 or more.
func (c AdminAPI) OK(method, path, body string) *httptest.ResponseRecorder {
	c.T.Helper()
	w := c.Call(method, path, body)
	if w.Code >= 300 {
		c.T.Fatalf("%s %s = %d %s", method, path, w.Code, w.Body)
	}
	return w
}

// APIError is the error body of docs/06 section 1.
type APIError struct {
	Error struct {
		Code   string
		Fields map[string]string
	}
}

// ErrorOf decodes the error body of a response.
func ErrorOf(t *testing.T, w *httptest.ResponseRecorder) APIError {
	t.Helper()
	return Decode[APIError](t, w)
}

// Expect fails the test unless the call answers status with the error code.
func (c AdminAPI) Expect(method, path, body string, status int, code string) APIError {
	c.T.Helper()
	w := c.Call(method, path, body)
	e := ErrorOf(c.T, w)
	if w.Code != status || (code != "" && e.Error.Code != code) {
		c.T.Fatalf("%s %s = %d %s, want %d %s", method, path, w.Code, w.Body, status, code)
	}
	return e
}

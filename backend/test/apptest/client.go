package apptest

import (
	"net/http/httptest"
	"testing"
)

// Client calls the real API with one session's cookies. Paths start after /v1.
type Client struct {
	*App
	Session Session
}

// As returns a client for a signed-in session.
func (a *App) As(s Session) Client { return Client{App: a, Session: s} }

// AsAdmin signs in the super admin (authenticator check passed) and returns a client.
func (a *App) AsAdmin() Client { return a.As(a.Admin()) }

func (c Client) Call(method, path, body string) *httptest.ResponseRecorder {
	return c.App.Call(method, "/v1"+path, body, c.Session.Cookies()...)
}

// CallWith is Call with extra request headers.
func (c Client) CallWith(method, path, body string, headers map[string]string) *httptest.ResponseRecorder {
	return c.App.CallWith(DefaultIP, method, "/v1"+path, body, headers, c.Session.Cookies()...)
}

// OK calls and fails the test on any answer of 300 or more.
func (c Client) OK(method, path, body string) *httptest.ResponseRecorder {
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
func (c Client) Expect(method, path, body string, status int, code string) APIError {
	c.T.Helper()
	w := c.Call(method, path, body)
	e := ErrorOf(c.T, w)
	if w.Code != status || (code != "" && e.Error.Code != code) {
		c.T.Fatalf("%s %s = %d %s, want %d %s", method, path, w.Code, w.Body, status, code)
	}
	return e
}

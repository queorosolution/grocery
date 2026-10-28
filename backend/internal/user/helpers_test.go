package user_test

import (
	"net/http"
	"testing"

	"github.com/google/uuid"

	"gp/internal/user"
	"gp/test/apptest"
	"gp/test/fakes"
)

type fixture struct {
	a  *apptest.App
	me apptest.Client // the first customer
}

func newFixture(t *testing.T) fixture {
	t.Helper()
	a := apptest.New(t)
	return fixture{a: a, me: a.As(a.SignedIn(fakes.Claims{Sub: "nimal", Email: "nimal@example.com", Name: "Nimal"}))}
}

// another signs in a second customer.
func (f fixture) another(name string) apptest.Client {
	return f.a.As(f.a.SignedIn(fakes.Claims{Sub: name, Email: name + "@example.com", Name: name}))
}

const colomboPin = `"lat":6.93,"lng":79.87` // about 1.2 km from the seeded shop

func addressBody(label string, extra string) string {
	body := `{"label":"` + label + `","line1":"12 Galle Road, Colombo 03",` + colomboPin
	if extra != "" {
		body += "," + extra
	}
	return body + "}"
}

func (f fixture) addAddress(c apptest.Client, label, extra string) user.AddressResult {
	f.a.T.Helper()
	w := c.OK(http.MethodPost, "/me/addresses", addressBody(label, extra))
	return apptest.Decode[user.AddressResult](f.a.T, w)
}

// shopWithRule makes an ACTIVE shop at the seed location and a platform fee rule
// that reaches 8 km by road.
func (f fixture) shopWithRule(slug string) uuid.UUID {
	id := f.a.SeedShop(slug)
	f.a.Exec(`UPDATE shops SET status = 'ACTIVE' WHERE id = $1`, id)
	f.a.Exec(`INSERT INTO delivery_fee_rules (shop_id) SELECT NULL WHERE NOT EXISTS (SELECT 1 FROM delivery_fee_rules WHERE shop_id IS NULL)`)
	return id
}

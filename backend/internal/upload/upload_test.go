package upload_test

import (
	"net/http"
	"net/url"
	"strconv"
	"strings"
	"testing"

	"github.com/google/uuid"

	"gp/internal/upload"
	"gp/test/apptest"
	"gp/test/fakes"
)

const presign = "/uploads/presign"

func body(purpose, contentType string, size int64, shop *uuid.UUID) string {
	b := `{"purpose":"` + purpose + `","contentType":"` + contentType + `","size":` + strconv.FormatInt(size, 10)
	if shop != nil {
		b += `,"shopId":"` + shop.String() + `"`
	}
	return b + `}`
}

func TestPresign_AdminGetsALockedLink(t *testing.T) {
	a := apptest.New(t)
	admin := a.AsAdmin()

	res := apptest.Decode[upload.Result](t, admin.OK(http.MethodPost, presign, body("PRODUCT_IMAGE", "image/webp", 183000, nil)))
	u, err := url.Parse(res.UploadURL)
	if err != nil {
		t.Fatal(err)
	}
	key := strings.TrimPrefix(u.Path, "/test-bucket/")
	if u.Scheme != "https" || u.Host != "test-account.r2.cloudflarestorage.com" ||
		!strings.HasPrefix(key, "PRODUCT_IMAGE/") || !strings.HasSuffix(key, ".webp") {
		t.Fatalf("upload url = %s", u.Redacted())
	}
	if res.PublicURL != apptest.ImgBase+"/"+key || res.ExpiresIn != 300 {
		t.Fatalf("public url %q, expires %d", res.PublicURL, res.ExpiresIn)
	}
	q := u.Query()
	signed := q.Get("X-Amz-SignedHeaders")
	for _, h := range []string{"content-type", "content-length", "host"} {
		if !strings.Contains(signed, h) {
			t.Errorf("signed headers %q miss %s", signed, h)
		}
	}
	if q.Get("X-Amz-Expires") != "300" || q.Get("X-Amz-Signature") == "" {
		t.Errorf("query = %v", q)
	}
	if strings.Contains(res.UploadURL, "test-secret-key") {
		t.Fatal("the secret key is in the link")
	}
	if again := apptest.Decode[upload.Result](t, admin.OK(http.MethodPost, presign, body("SHOP_LOGO", "image/png", 1, nil))); again.PublicURL == res.PublicURL {
		t.Fatal("two uploads got the same address")
	}
}

// XSS-07: nothing that a browser could run is ever presigned.
func TestPresign_OnlyPlainImages(t *testing.T) {
	admin := apptest.New(t).AsAdmin()
	for _, ct := range []string{"image/svg+xml", "text/html", "application/javascript", "image/gif", "image/jpeg; charset=x", "IMAGE/PNG", ""} {
		admin.Expect(http.MethodPost, presign, body("PRODUCT_IMAGE", ct, 1000, nil), http.StatusBadRequest, "UPLOAD_REJECTED")
	}
	for _, ct := range []string{"image/jpeg", "image/png", "image/webp"} {
		admin.OK(http.MethodPost, presign, body("PRODUCT_IMAGE", ct, 1000, nil))
	}
}

func TestPresign_SizeAndPurpose(t *testing.T) {
	admin := apptest.New(t).AsAdmin()
	admin.OK(http.MethodPost, presign, body("PRODUCT_IMAGE", "image/png", upload.MaxSize, nil))
	admin.Expect(http.MethodPost, presign, body("PRODUCT_IMAGE", "image/png", upload.MaxSize+1, nil), http.StatusBadRequest, "UPLOAD_REJECTED")
	for name, b := range map[string]string{
		"zero size":       body("PRODUCT_IMAGE", "image/png", 0, nil),
		"negative size":   body("PRODUCT_IMAGE", "image/png", -5, nil),
		"unknown purpose": body("AVATAR", "image/png", 10, nil),
		"key from client": `{"purpose":"PRODUCT_IMAGE","contentType":"image/png","size":10,"key":"../x"}`,
	} {
		t.Run(name, func(t *testing.T) {
			admin.Expect(http.MethodPost, presign, b, http.StatusBadRequest, "VALIDATION_ERROR")
		})
	}
}

func TestPresign_WhoMayUpload(t *testing.T) {
	a := apptest.New(t)
	alpha, beta := a.SeedShop("alpha"), a.SeedShop("beta")
	member := func(sub, role string, shop uuid.UUID) apptest.Client {
		s := a.SignedIn(fakes.Claims{Sub: sub, Email: sub + "@example.test"})
		a.Grant(s.Me.ID, &shop, role)
		return a.As(s)
	}
	owner, staff := member("owner", "SHOP_OWNER", alpha), member("staff", "SHOP_STAFF", alpha)
	customer := a.As(a.SignedIn(fakes.Claims{Sub: "customer", Email: "c@example.test"}))
	admin := a.AsAdmin()

	owner.OK(http.MethodPost, presign, body("PRODUCT_IMAGE", "image/png", 10, &alpha))
	admin.OK(http.MethodPost, presign, body("PRODUCT_IMAGE", "image/png", 10, &beta))
	owner.Expect(http.MethodPost, presign, body("PRODUCT_IMAGE", "image/png", 10, &beta), http.StatusNotFound, "NOT_FOUND")
	owner.Expect(http.MethodPost, presign, body("PRODUCT_IMAGE", "image/png", 10, nil), http.StatusForbidden, "FORBIDDEN")
	owner.Expect(http.MethodPost, presign, body("SHOP_LOGO", "image/png", 10, &alpha), http.StatusBadRequest, "VALIDATION_ERROR")
	customer.Expect(http.MethodPost, presign, body("PRODUCT_IMAGE", "image/png", 10, nil), http.StatusForbidden, "FORBIDDEN")
	customer.Expect(http.MethodPost, presign, body("PRODUCT_IMAGE", "image/png", 10, &alpha), http.StatusNotFound, "NOT_FOUND")

	staff.Expect(http.MethodPost, presign, body("PRODUCT_IMAGE", "image/png", 10, &alpha), http.StatusForbidden, "FORBIDDEN")
	a.Exec(`UPDATE memberships SET can_edit_products = true WHERE user_id = $1`, staff.Session.Me.ID)
	staff.OK(http.MethodPost, presign, body("PRODUCT_IMAGE", "image/png", 10, &alpha))
}

func TestPresign_NeedsASignedInUser(t *testing.T) {
	a := apptest.New(t)
	w := a.Call(http.MethodPost, "/v1"+presign, body("PRODUCT_IMAGE", "image/png", 10, nil))
	if w.Code != http.StatusUnauthorized {
		t.Fatalf("status = %d", w.Code)
	}
}

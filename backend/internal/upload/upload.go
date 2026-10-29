// Package upload hands out short-lived links so the browser can put an image
// straight into R2. The API never touches the file (docs/06 section 11).
package upload

import (
	"context"
	"strings"
	"time"

	"github.com/google/uuid"

	"gp/internal/shared/apperr"
	"gp/internal/shared/validate"
)

// Purpose says what an image is for. It is the first part of the stored key.
type Purpose string

const (
	ProductImage Purpose = "PRODUCT_IMAGE"
	ShopLogo     Purpose = "SHOP_LOGO"
)

// MaxSize is the largest image, 2 MB.
const MaxSize = 2 << 20

// TTL is how long an upload link works.
const TTL = 5 * time.Minute

// extensions are the only image types we serve. SVG and HTML are left out on
// purpose: from our own image domain they could run script (XSS-07).
var extensions = map[string]string{"image/jpeg": "jpg", "image/png": "png", "image/webp": "webp"}

// Signer makes a PUT link locked to one key, type and length.
type Signer interface {
	PresignPut(ctx context.Context, key, contentType string, size int64, ttl time.Duration) (string, error)
}

type Request struct {
	Purpose     Purpose `json:"purpose"`
	ContentType string  `json:"contentType"`
	Size        int64   `json:"size"`
	// ShopID is set when a shop's team member uploads; an admin leaves it out.
	ShopID *uuid.UUID `json:"shopId"`
}

type Result struct {
	UploadURL string `json:"uploadUrl"`
	PublicURL string `json:"publicUrl"`
	ExpiresIn int    `json:"expiresIn"`
}

type Service struct {
	signer  Signer
	imgBase string // IMG_BASE_URL
}

func NewService(signer Signer, imgBaseURL string) *Service {
	return &Service{signer: signer, imgBase: strings.TrimRight(imgBaseURL, "/")}
}

// Presign checks the request and returns the upload link and the address the
// image will have. The key is made here, never taken from the client.
func (s *Service) Presign(ctx context.Context, in Request) (Result, error) {
	if err := validate.New().
		Check("purpose", in.Purpose == ProductImage || in.Purpose == ShopLogo, "invalid").
		Int("size", in.Size, 1, 1<<40).Err(); err != nil {
		return Result{}, err
	}
	ext, ok := extensions[in.ContentType]
	if !ok || in.Size > MaxSize {
		return Result{}, apperr.New(apperr.UploadRejected)
	}
	key := string(in.Purpose) + "/" + uuid.NewString() + "." + ext
	url, err := s.signer.PresignPut(ctx, key, in.ContentType, in.Size, TTL)
	if err != nil {
		return Result{}, err
	}
	return Result{UploadURL: url, PublicURL: s.imgBase + "/" + key, ExpiresIn: int(TTL.Seconds())}, nil
}

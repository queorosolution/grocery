package upload

import (
	"net/http"

	"github.com/google/uuid"

	"gp/internal/shared/apperr"
	"gp/internal/shared/authn"
	"gp/internal/shared/authz"
	"gp/internal/shared/db"
	"gp/internal/shared/enum"
	"gp/internal/shared/routex"
)

type Handler struct {
	svc         *Service
	memberships authz.Loader
}

func NewHandler(svc *Service, memberships authz.Loader) *Handler {
	return &Handler{svc: svc, memberships: memberships}
}

// Presign handles POST /uploads/presign. The route needs a signed-in user; who
// may upload depends on the body, so it is decided here with the same checks
// the guards use: an admin without shopId, or a team member with product.edit
// for the shop in shopId.
func (h *Handler) Presign(w http.ResponseWriter, r *http.Request) {
	var in Request
	routex.User(w, r, http.StatusOK, false, &in, func(r *http.Request, _ db.Scope, _ uuid.UUID) (any, error) {
		if err := h.authorize(r, in); err != nil {
			return nil, err
		}
		return h.svc.Presign(r.Context(), in)
	})
}

func (h *Handler) authorize(r *http.Request, in Request) error {
	actor, ok := authn.From(r.Context())
	if !ok || h.memberships == nil {
		return apperr.New(apperr.Forbidden)
	}
	ms, err := h.memberships.Memberships(r.Context(), actor.UserID)
	if err != nil {
		return err
	}
	if in.ShopID == nil {
		_, err = authz.RequireAdmin(actor, ms)
		return err
	}
	if in.Purpose != ProductImage {
		return apperr.Validation(map[string]string{"purpose": "not_allowed"})
	}
	_, err = authz.Require(actor, ms, *in.ShopID, enum.PermissionProductEdit)
	return err
}

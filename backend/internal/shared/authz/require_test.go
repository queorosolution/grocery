package authz

import (
	"errors"
	"testing"

	"github.com/google/uuid"

	"gp/internal/shared/apperr"
	"gp/internal/shared/authn"
	"gp/internal/shared/enum"
)

func TestEveryConditionIsUnderstood(t *testing.T) {
	for role, perms := range conditionalPermissions {
		for perm, cond := range perms {
			if !conditionHolds(cond, Membership{Role: role, CanEditProducts: true}) {
				t.Errorf("%s / %s: condition %q is not understood by conditionHolds", role, perm, cond)
			}
		}
	}
}

func TestMembershipHas(t *testing.T) {
	staff := Membership{Role: enum.RoleShopStaff}
	if staff.Has(enum.PermissionProductEdit) {
		t.Error("staff without the flag must not edit products")
	}
	staff.CanEditProducts = true
	if !staff.Has(enum.PermissionProductEdit) {
		t.Error("staff with the flag must edit products")
	}
	if staff.Has(enum.PermissionTeamManage) {
		t.Error("staff must not manage the team")
	}
}

func TestRequire(t *testing.T) {
	shopA, shopB := uuid.New(), uuid.New()
	user := uuid.New()
	owner := Membership{Role: enum.RoleShopOwner, ShopID: shopA}
	staff := Membership{Role: enum.RoleShopStaff, ShopID: shopA}
	driver := Membership{Role: enum.RoleShopDriver, ShopID: shopA}
	admin := Membership{Role: enum.RoleSuperAdmin}
	actor := authn.Actor{UserID: user}
	withTotp := authn.Actor{UserID: user, TotpOK: true}

	tests := []struct {
		name string
		a    authn.Actor
		ms   []Membership
		shop uuid.UUID
		perm enum.Permission
		want apperr.Code // empty = allowed
	}{
		{"owner on own shop", actor, []Membership{owner}, shopA, enum.PermissionTeamManage, ""},
		{"owner on another shop is 404", actor, []Membership{owner}, shopB, enum.PermissionTeamManage, apperr.NotFound},
		{"staff cannot manage team", actor, []Membership{staff}, shopA, enum.PermissionTeamManage, apperr.Forbidden},
		{"no memberships on a shop is 404", actor, nil, shopA, enum.PermissionOrderReview, apperr.NotFound},
		{"admin without totp", actor, []Membership{admin}, shopA, enum.PermissionTeamManage, apperr.TotpRequired},
		{"admin with totp on any shop", withTotp, []Membership{admin}, shopB, enum.PermissionTeamManage, ""},
		{"admin lacks request_refund", withTotp, []Membership{admin}, shopA, enum.PermissionOrderRequestRefund, apperr.NotFound},
		{"driver route without shop", actor, []Membership{driver}, uuid.Nil, enum.PermissionOrderDeliver, ""},
		{"owner on a driver route", actor, []Membership{owner}, uuid.Nil, enum.PermissionOrderDeliver, apperr.Forbidden},
		{"nobody on a driver route", actor, nil, uuid.Nil, enum.PermissionOrderDeliver, apperr.Forbidden},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			scope, err := Require(tt.a, tt.ms, tt.shop, tt.perm)
			if tt.want == "" {
				if err != nil {
					t.Fatalf("denied: %v", err)
				}
				if scope.UserID != user {
					t.Fatalf("scope user = %v", scope.UserID)
				}
				return
			}
			var ae *apperr.Error
			if !errors.As(err, &ae) || ae.Code != tt.want {
				t.Fatalf("err = %v, want %s", err, tt.want)
			}
		})
	}
}

func TestRequireScopeShape(t *testing.T) {
	shop, user := uuid.New(), uuid.New()
	s, _ := Require(authn.Actor{UserID: user}, []Membership{{Role: enum.RoleShopOwner, ShopID: shop}}, shop, enum.PermissionOrderReview)
	if s.Privileged || s.ShopID != shop || s.Role != enum.RoleShopOwner {
		t.Fatalf("owner scope = %+v", s)
	}
	s, _ = Require(authn.Actor{UserID: user, TotpOK: true}, []Membership{{Role: enum.RoleSuperAdmin}}, shop, enum.PermissionOrderReview)
	if !s.Privileged || s.ShopID != shop {
		t.Fatalf("admin scope = %+v", s)
	}
	s, _ = Require(authn.Actor{UserID: user}, []Membership{{Role: enum.RoleShopDriver, ShopID: shop}}, uuid.Nil, enum.PermissionOrderDeliver)
	if s.ShopID != uuid.Nil || s.Privileged {
		t.Fatalf("driver scope = %+v", s)
	}
}

func TestRequireAdmin(t *testing.T) {
	user := uuid.New()
	admin := []Membership{{Role: enum.RoleSuperAdmin}}
	if _, err := RequireAdmin(authn.Actor{UserID: user, TotpOK: true}, admin); err != nil {
		t.Fatalf("admin with totp denied: %v", err)
	}
	if _, err := RequireAdmin(authn.Actor{UserID: user}, admin); !apperr.Is(err, apperr.TotpRequired) {
		t.Fatalf("admin without totp = %v", err)
	}
	owner := []Membership{{Role: enum.RoleShopOwner, ShopID: uuid.New()}}
	if _, err := RequireAdmin(authn.Actor{UserID: user, TotpOK: true}, owner); !apperr.Is(err, apperr.Forbidden) {
		t.Fatalf("owner on admin route = %v", err)
	}
}

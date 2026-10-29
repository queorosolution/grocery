package authz

import (
	"testing"

	"gp/internal/shared/enum"
)

func TestRoleHas(t *testing.T) {
	if !RoleHas(enum.RoleShopOwner, enum.PermissionOrderReview) {
		t.Error("shop owner should review orders")
	}
	if RoleHas(enum.RoleCustomer, enum.PermissionOrderReview) {
		t.Error("customer must hold no permission")
	}
	if RoleHas(enum.RoleShopStaff, enum.PermissionProductEdit) {
		t.Error("staff product.edit is conditional, RoleHas must say no")
	}
}

func TestCondition(t *testing.T) {
	if _, ok := Condition(enum.RoleShopStaff, enum.PermissionProductEdit); !ok {
		t.Error("staff product.edit should carry a condition")
	}
	if _, ok := Condition(enum.RoleShopOwner, enum.PermissionProductEdit); ok {
		t.Error("owner product.edit has no condition")
	}
}

func TestEveryRoleIsListed(t *testing.T) {
	for _, r := range enum.RoleValues() {
		if _, ok := rolePermissions[r]; !ok {
			t.Errorf("role %s has no entry in role_permissions", r)
		}
	}
}

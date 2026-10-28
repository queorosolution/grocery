package main

import (
	"bytes"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

const contractsDir = "../../../contracts"

func realContract(t *testing.T) *Contract {
	t.Helper()
	c, err := loadContract(filepath.Join(contractsDir, "enums.json"))
	if err != nil {
		t.Fatal(err)
	}
	return c
}

func TestPascal(t *testing.T) {
	for in, want := range map[string]string{
		"SUPER_ADMIN":          "SuperAdmin",
		"order.status_changed": "OrderStatusChanged",
		"PaymentStatus":        "PaymentStatus",
		"WA_LINK":              "WaLink",
		"TOTP_REQUIRED":        "TotpRequired",
	} {
		if got := pascal(in); got != want {
			t.Errorf("pascal(%q) = %q, want %q", in, got, want)
		}
	}
}

func TestGenerateIsRepeatable(t *testing.T) {
	c := realContract(t)
	be1, web1, err := generate(c)
	if err != nil {
		t.Fatal(err)
	}
	be2, web2, _ := generate(c)
	all1, all2 := append(be1, web1...), append(be2, web2...)
	for i := range all1 {
		if !bytes.Equal(all1[i].Data, all2[i].Data) {
			t.Errorf("%s differs between two runs", all1[i].Path)
		}
	}
}

// The committed *_gen files must match the contract: run `make gen` after editing it.
func TestGeneratedFilesAreUpToDate(t *testing.T) {
	be, web, err := generate(realContract(t))
	if err != nil {
		t.Fatal(err)
	}
	for root, files := range map[string][]output{"../..": be, "../../../web": web} {
		for _, f := range files {
			got, err := os.ReadFile(filepath.Join(root, f.Path))
			if err != nil {
				t.Fatalf("%s: %v (run make gen)", f.Path, err)
			}
			if !bytes.Equal(got, f.Data) {
				t.Errorf("%s is out of date (run make gen)", f.Path)
			}
		}
	}
}

func TestGeneratedContent(t *testing.T) {
	be, web, _ := generate(realContract(t))
	text := map[string]string{}
	for _, f := range append(be, web...) {
		text[filepath.Base(f.Path)] = strings.Join(strings.Fields(string(f.Data)), " ") // gofmt aligns columns
	}
	for file, snippets := range map[string][]string{
		"enum_gen.go":   {`RoleSuperAdmin Role = "SUPER_ADMIN"`, "func OrderStatusValues()", `PermissionOrderReview Permission = "order.review"`},
		"perm_gen.go":   {"func RoleHas", "can_edit_products"},
		"flow_gen.go":   {`ID: "T19"`, "func IsFinal", "func Allowed("},
		"codes_gen.go":  {`CodeSlotFull Code = "SLOT_FULL"`, "return 423"},
		"enums.gen.ts":  {`export const roleValues = ["SUPER_ADMIN"`, "export type OrderStatus"},
		"perms.gen.ts":  {"Record<Role, readonly Permission[]>"},
		"errors.gen.ts": {`SLOT_FULL: { http: 409`, "export type ErrorCode"},
	} {
		for _, s := range snippets {
			if !strings.Contains(text[file], s) {
				t.Errorf("%s misses %q", file, s)
			}
		}
	}
}

func TestValidateRejectsBadContracts(t *testing.T) {
	base := func() *Contract {
		from := "A"
		return &Contract{
			Enums:              map[string][]string{"Role": {"X"}, "Permission": {"p.q"}, "OrderStatus": {"A", "B"}},
			RolePermissions:    map[string][]string{"X": {"p.q"}},
			OrderTransitions:   []Transition{{ID: "T1", From: &from, To: "B", Actors: []string{"X"}}},
			FinalOrderStatuses: []string{"B"},
			ErrorCodes:         map[string]ErrorCode{"INTERNAL": {HTTP: 500, Message: "m"}},
		}
	}
	if err := base().validate(); err != nil {
		t.Fatalf("base contract should be valid: %v", err)
	}
	cases := map[string]func(*Contract){
		"duplicate value":     func(c *Contract) { c.Enums["Role"] = []string{"X", "X"} },
		"unknown role":        func(c *Contract) { c.RolePermissions["Y"] = nil },
		"unknown permission":  func(c *Contract) { c.RolePermissions["X"] = []string{"nope"} },
		"leaves final status": func(c *Contract) { b := "B"; c.OrderTransitions[0].From = &b; c.OrderTransitions[0].To = "A" },
		"unknown actor":       func(c *Contract) { c.OrderTransitions[0].Actors = []string{"Z"} },
		"bad http status":     func(c *Contract) { c.ErrorCodes["INTERNAL"] = ErrorCode{HTTP: 200, Message: "m"} },
		"missing INTERNAL":    func(c *Contract) { c.ErrorCodes = map[string]ErrorCode{} },
	}
	for name, mutate := range cases {
		c := base()
		mutate(c)
		if err := c.validate(); err == nil {
			t.Errorf("%s: expected an error", name)
		}
	}
}

func TestEnumNameCollisionIsAnError(t *testing.T) {
	c := &Contract{Enums: map[string][]string{"Role": {"A_B", "A.B"}}}
	if _, err := emitEnums(c); err == nil {
		t.Fatal("expected a collision error")
	}
}

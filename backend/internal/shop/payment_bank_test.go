package shop_test

import (
	"net/http"
	"strings"
	"testing"

	"gp/internal/shop"
	"gp/test/apptest"
)

const bankBody = `"bank":{"bankName":"Commercial Bank","accountName":"Krishna Super","accountNumber":"8001234567","branch":"Galle Road"}`

// A MANUAL account carries the bank details the customer is shown (docs/06 payment response).
func TestManualAccount_Bank(t *testing.T) {
	f := newFixture(t)
	acc := apptest.Decode[shop.Account](t, f.OK(http.MethodPost, "/admin/payment-accounts", `{"ownerType":"PLATFORM","provider":"MANUAL",`+bankBody+`}`))
	if acc.Bank == nil || acc.Bank.AccountNumber != "8001234567" || acc.Bank.Branch != "Galle Road" {
		t.Fatalf("bank = %+v", acc.Bank)
	}
	// It comes back from the list, and a patch replaces it.
	list := f.OK(http.MethodGet, "/admin/payment-accounts", "").Body.String()
	if !strings.Contains(list, `"accountNumber":"8001234567"`) {
		t.Errorf("list = %s", list)
	}
	patched := apptest.Decode[shop.Account](t, f.OK(http.MethodPatch, "/admin/payment-accounts/"+acc.ID.String(),
		`{"bank":{"bankName":"Sampath","accountName":"Krishna Super","accountNumber":"9990001","branch":""}}`))
	if patched.Bank == nil || patched.Bank.BankName != "Sampath" || patched.Bank.Branch != "" {
		t.Errorf("patched bank = %+v", patched.Bank)
	}
	// An account made without a bank shows null, and can be given one later.
	bare := apptest.Decode[shop.Account](t, f.OK(http.MethodPost, "/admin/payment-accounts", `{"ownerType":"PLATFORM","provider":"MANUAL"}`))
	if bare.Bank != nil {
		t.Errorf("bank = %+v, want none", bare.Bank)
	}
	if n := f.a.Count(`SELECT count(*) FROM audit_log WHERE action = 'payment_account.update' AND after->'bank'->>'bankName' = 'Sampath'`); n != 1 {
		t.Errorf("bank change audited %d times, want 1", n)
	}
}

func TestManualAccount_BankRefused(t *testing.T) {
	f := newFixture(t)
	for name, tc := range map[string]struct{ body, field string }{
		"no account number":  {`{"ownerType":"PLATFORM","provider":"MANUAL","bank":{"bankName":"B","accountName":"A","accountNumber":"","branch":""}}`, "bank.accountNumber"},
		"no bank name":       {`{"ownerType":"PLATFORM","provider":"MANUAL","bank":{"bankName":"","accountName":"A","accountNumber":"1","branch":""}}`, "bank.bankName"},
		"spaces around name": {`{"ownerType":"PLATFORM","provider":"MANUAL","bank":{"bankName":"B","accountName":" A","accountNumber":"1","branch":""}}`, "bank.accountName"},
		"long branch":        {`{"ownerType":"PLATFORM","provider":"MANUAL","bank":{"bankName":"B","accountName":"A","accountNumber":"1","branch":"` + strings.Repeat("x", 81) + `"}}`, "bank.branch"},
		"bank on PayHere":    {`{"ownerType":"PLATFORM","provider":"PAYHERE","merchantId":"1230001","merchantSecret":"` + secretValue + `",` + bankBody + `}`, "bank"},
	} {
		w := f.Call(http.MethodPost, "/admin/payment-accounts", tc.body)
		if e := apptest.ErrorOf(t, w); w.Code != http.StatusBadRequest || e.Error.Fields[tc.field] == "" {
			t.Errorf("%s: %d %s, want 400 on %s", name, w.Code, w.Body, tc.field)
		}
	}
	// A PayHere account takes no bank on a patch either, and a manual one none of the credentials.
	ph := f.payhere(payhereBody)
	if w := f.Call(http.MethodPatch, "/admin/payment-accounts/"+ph.ID.String(), `{`+bankBody+`}`); w.Code != http.StatusBadRequest {
		t.Errorf("bank on a PayHere patch = %d", w.Code)
	}
}

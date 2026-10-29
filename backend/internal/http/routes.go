package http

import (
	nethttp "net/http"

	"gp/internal/shared/enum"
)

// Route is one line of the route table. Every route has a guard; a route
// without one fails TestEveryRouteHasValidGuard.
type Route struct {
	Method  string
	Pattern string
	Guard   Guard
	Handler nethttp.HandlerFunc
}

// Routes lists every API route. Patterns are relative to the /v1 base. It is
// exported so the security tests can walk the same table the server serves.
func Routes(d Deps) []Route {
	table := []Route{
		{nethttp.MethodGet, "/health", Public, health},
		{nethttp.MethodGet, "/bootstrap", Public, bootstrap(d)},
	}
	if d.Auth != nil {
		table = append(table,
			Route{nethttp.MethodPost, "/auth/google", Public, d.Auth.Google},
			Route{nethttp.MethodPost, "/auth/refresh", Public, d.Auth.Refresh},
			Route{nethttp.MethodPost, "/auth/logout", User, d.Auth.Logout},
			Route{nethttp.MethodGet, "/me", User, d.Auth.Me},
			Route{nethttp.MethodPost, "/auth/totp/setup", User, d.Auth.TotpSetup},
			Route{nethttp.MethodPost, "/auth/totp/verify", User, d.Auth.TotpVerify},
			Route{nethttp.MethodGet, "/admin/users", Admin, d.Auth.Users},
			Route{nethttp.MethodPatch, "/admin/users/{id}", Admin, d.Auth.SetUserStatus},
		)
	}
	if d.Team != nil {
		const team = "/console/shops/{shopId}/team"
		manage := Perm(enum.PermissionTeamManage)
		table = append(table,
			Route{nethttp.MethodGet, team, manage, d.Team.List},
			Route{nethttp.MethodPost, team + "/invites", manage, d.Team.Invite},
			Route{nethttp.MethodPatch, team + "/{membershipId}", manage, d.Team.Change},
			Route{nethttp.MethodDelete, team + "/{membershipId}", manage, d.Team.Remove},
			Route{nethttp.MethodPost, "/admin/invites", Admin, d.Team.AdminInvite},
			Route{nethttp.MethodDelete, "/admin/invites/{id}", Admin, d.Team.AdminRevokeInvite},
		)
	}
	if d.Shop != nil {
		table = append(table,
			Route{nethttp.MethodGet, "/shops", Public, d.Shop.PublicList},
			Route{nethttp.MethodGet, "/admin/shops", Admin, d.Shop.List},
			Route{nethttp.MethodPost, "/admin/shops", Admin, d.Shop.Create},
			Route{nethttp.MethodGet, "/admin/shops/{id}", Admin, d.Shop.Get},
			Route{nethttp.MethodPatch, "/admin/shops/{id}", Admin, d.Shop.Update},
			Route{nethttp.MethodPut, "/admin/shops/{id}/settings", Admin, d.Shop.PutSettings},
			Route{nethttp.MethodPut, "/admin/shops/{id}/hours", Admin, d.Shop.PutHours},
			Route{nethttp.MethodPost, "/admin/shops/{id}/accepting", Admin, d.Shop.Accepting},
			Route{nethttp.MethodGet, "/admin/shops/{id}/readiness", Admin, d.Shop.Readiness},
			Route{nethttp.MethodGet, "/admin/payment-accounts", Admin, d.Shop.Accounts},
			Route{nethttp.MethodPost, "/admin/payment-accounts", Admin, d.Shop.CreateAccount},
			Route{nethttp.MethodPatch, "/admin/payment-accounts/{id}", Admin, d.Shop.UpdateAccount},
		)
	}
	if d.Delivery != nil {
		table = append(table,
			Route{nethttp.MethodGet, "/shops/{slug}/slots", Public, d.Delivery.PublicSlots},
			Route{nethttp.MethodGet, "/admin/fee-rules", Admin, d.Delivery.Rules},
			Route{nethttp.MethodPut, "/admin/fee-rules/default", Admin, d.Delivery.SetDefaultRule},
			Route{nethttp.MethodPut, "/admin/fee-rules/shops/{id}", Admin, d.Delivery.SetShopRule},
			Route{nethttp.MethodPost, "/admin/fee-rules/preview", Admin, d.Delivery.Preview},
			Route{nethttp.MethodGet, "/admin/shops/{id}/slots", Admin, d.Delivery.AdminSlots},
			Route{nethttp.MethodPost, "/admin/shops/{id}/slots", Admin, d.Delivery.CreateSlots},
			Route{nethttp.MethodPost, "/admin/shops/{id}/slots/copy-week", Admin, d.Delivery.CopyWeek},
			Route{nethttp.MethodPatch, "/admin/slots/{id}", Admin, d.Delivery.UpdateSlot},
		)
	}
	if d.Audit != nil {
		table = append(table, Route{nethttp.MethodGet, "/admin/audit", Admin, d.Audit.List})
	}
	return table
}

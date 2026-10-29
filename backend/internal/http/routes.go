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

// TriggerPattern is the signed call of a shop's own system (docs/06 section 7).
const TriggerPattern = "/connectors/{id}/trigger"

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
			Route{nethttp.MethodPost, "/admin/shops/{id}/cart-tenant", Admin, d.Shop.EnableCart},
			Route{nethttp.MethodPost, "/admin/shops/{id}/cart-sync", Admin, d.Shop.ResyncCart},
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
	if d.User != nil {
		table = append(table,
			Route{nethttp.MethodPatch, "/me", User, d.User.SetName},
			Route{nethttp.MethodPost, "/me/phone/send", User, d.User.SendPhoneCode},
			Route{nethttp.MethodPost, "/me/phone/verify", User, d.User.VerifyPhoneCode},
			Route{nethttp.MethodGet, "/me/addresses", User, d.User.Addresses},
			Route{nethttp.MethodPost, "/me/addresses", User, d.User.AddAddress},
			Route{nethttp.MethodPatch, "/me/addresses/{id}", User, d.User.UpdateAddress},
			Route{nethttp.MethodDelete, "/me/addresses/{id}", User, d.User.DeleteAddress},
		)
	}
	if d.Catalog != nil {
		const products = "/console/shops/{shopId}/products"
		table = append(table,
			Route{nethttp.MethodGet, "/admin/categories", Admin, d.Catalog.Categories},
			Route{nethttp.MethodPost, "/admin/categories", Admin, d.Catalog.CreateCategory},
			Route{nethttp.MethodPatch, "/admin/categories/{id}", Admin, d.Catalog.UpdateCategory},
			Route{nethttp.MethodGet, "/admin/products", Admin, d.Catalog.Products},
			Route{nethttp.MethodPost, "/admin/products", Admin, d.Catalog.CreateProduct},
			Route{nethttp.MethodPatch, "/admin/products/{id}", Admin, d.Catalog.UpdateProduct},
			Route{nethttp.MethodPost, "/admin/shops/{id}/products", Admin, d.Catalog.AddShopProduct},
			Route{nethttp.MethodGet, products, Perm(enum.PermissionOrderReview), d.Catalog.ShopProducts},
			Route{nethttp.MethodPatch, products + "/{id}", Perm(enum.PermissionProductEdit), d.Catalog.UpdateShopProduct},
			Route{nethttp.MethodGet, "/home", Public, d.Catalog.Home},
			Route{nethttp.MethodGet, "/search", Public, d.Catalog.Search},
			Route{nethttp.MethodGet, "/categories", Public, d.Catalog.CategoryTree},
			Route{nethttp.MethodGet, "/shops/{slug}", Public, d.Catalog.ShopPage},
			Route{nethttp.MethodGet, "/shops/{slug}/products", Public, d.Catalog.PublicProducts},
			Route{nethttp.MethodGet, "/shops/{slug}/buy-again", User, d.Catalog.BuyAgain},
		)
	}
	if d.Pricing != nil {
		table = append(table,
			Route{nethttp.MethodGet, "/admin/shops/{id}/category-markups", Admin, d.Pricing.Markups},
			Route{nethttp.MethodPut, "/admin/shops/{id}/category-markups", Admin, d.Pricing.SetMarkups},
		)
	}
	if d.Connector != nil {
		table = append(table,
			Route{nethttp.MethodPost, "/admin/shops/{id}/connector", Admin, d.Connector.Create},
			Route{nethttp.MethodGet, "/admin/shops/{id}/connector", Admin, d.Connector.ForShop},
			Route{nethttp.MethodPut, "/admin/connectors/{id}/secret", Admin, d.Connector.ReplaceSecret},
			Route{nethttp.MethodPost, "/admin/connectors/{id}/test", Admin, d.Connector.Test},
			Route{nethttp.MethodGet, "/admin/connectors/{id}/tables", Admin, d.Connector.Tables},
			Route{nethttp.MethodGet, "/admin/connectors/{id}/columns", Admin, d.Connector.Columns},
			Route{nethttp.MethodPut, "/admin/connectors/{id}/mapping", Admin, d.Connector.SetMapping},
			Route{nethttp.MethodPost, "/admin/connectors/{id}/preview", Admin, d.Connector.Preview},
			Route{nethttp.MethodPost, "/admin/connectors/{id}/activate", Admin, d.Connector.Activate},
			Route{nethttp.MethodPost, "/admin/connectors/{id}/pause", Admin, d.Connector.Pause},
			Route{nethttp.MethodPost, "/admin/connectors/{id}/sync", Admin, d.Connector.Sync},
			Route{nethttp.MethodPost, "/admin/connectors/{id}/trigger-secret", Admin, d.Connector.TriggerSecret},
			Route{nethttp.MethodGet, "/admin/connectors/{id}/runs", Admin, d.Connector.Runs},
			Route{nethttp.MethodGet, "/admin/sync-issues", Admin, d.Connector.Issues},
			Route{nethttp.MethodPost, "/admin/sync-issues/{id}/resolve", Admin, d.Connector.Resolve},
			Route{nethttp.MethodPost, TriggerPattern, Sig, d.Connector.Trigger},
		)
	}
	if d.Upload != nil {
		table = append(table, Route{nethttp.MethodPost, "/uploads/presign", User, d.Upload.Presign})
	}
	if d.Audit != nil {
		table = append(table, Route{nethttp.MethodGet, "/admin/audit", Admin, d.Audit.List})
	}
	return table
}

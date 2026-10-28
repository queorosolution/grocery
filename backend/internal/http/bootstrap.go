package http

import (
	nethttp "net/http"

	"gp/internal/auth"
	"gp/internal/shared/config"
	"gp/internal/shared/httpx"
)

func health(w nethttp.ResponseWriter, _ *nethttp.Request) {
	httpx.JSON(w, nethttp.StatusOK, map[string]bool{"ok": true})
}

type bootstrapBody struct {
	Platform platformInfo `json:"platform"`
	Features featureInfo  `json:"features"`
	// Me is null when signed out.
	Me *auth.Me `json:"me"`
}

type platformInfo struct {
	Name           string  `json:"name"`
	Domain         string  `json:"domain"`
	SupportPhone   string  `json:"supportPhone"`
	SingleShopSlug *string `json:"singleShopSlug"`
}

type featureInfo struct {
	WhatsApp      bool `json:"whatsapp"`
	ManualPayment bool `json:"manualPayment"`
}

func bootstrap(d Deps) nethttp.HandlerFunc {
	return func(w nethttp.ResponseWriter, r *nethttp.Request) {
		body := bootstrapBody{
			Platform: platformFrom(d.Config),
			Features: featureInfo{WhatsApp: d.Config.FeatureWhatsApp, ManualPayment: d.Config.FeatureManualPay},
		}
		if d.Auth != nil {
			me, err := d.Auth.MeOrNil(r)
			if err != nil {
				httpx.Error(w, r, err)
				return
			}
			body.Me = me
		}
		httpx.JSON(w, nethttp.StatusOK, body)
	}
}

func platformFrom(c *config.Config) platformInfo {
	return platformInfo{Name: c.PlatformName, Domain: c.PlatformDomain, SupportPhone: c.SupportPhone}
}

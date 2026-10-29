// Package setup builds the notify service for the API and the worker, so both
// pick the channels the same way (docs/09 section 3.1): the SMS gateway in prod,
// the log channel in local and dev.
package setup

import (
	"gp/internal/notify"
	"gp/internal/notify/logchannel"
	"gp/internal/notify/sms"
	"gp/internal/notify/walink"
	"gp/internal/shared/config"
	"gp/internal/shared/db"
	"gp/internal/shared/secrets"
)

func Service(cfg *config.Config, d *db.DB, sec secrets.Getter) (*notify.Service, error) {
	var smsChannel notify.Channel = logchannel.New(cfg.IsLocal())
	if cfg.Env == config.EnvProd {
		gateway, err := sms.New(cfg.SMSAPIURL, cfg.SMSSenderID, sec)
		if err != nil {
			return nil, err
		}
		smsChannel = gateway
	}
	return notify.NewService(notify.Deps{DB: d, SMS: smsChannel, WALink: walink.New(), DailyCap: cfg.SMSDailyCap,
		AdminPhone: cfg.AdminAlertPhone, App: cfg.PlatformName}), nil
}

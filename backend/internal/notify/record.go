package notify

import (
	"context"
	"strconv"
	"strings"

	"gp/internal/shared/clock"
	"gp/internal/shared/db"
	"gp/internal/shared/enum"
	"gp/internal/shared/logx"
)

// The values of notifications.status.
const (
	statusSent    = "SENT"
	statusFailed  = "FAILED"
	statusSkipped = "SKIPPED"
	statusLink    = "LINK"
)

type outcome struct {
	status string
	ref    string
	reason string // why it failed or was skipped: a fixed word or a channel error without personal data
	cost   int
}

// Column limits of the notifications table.
const (
	maxRef    = 100
	maxReason = 300
)

func cut(s string, n int) string {
	if r := []rune(s); len(r) > n {
		return string(r[:n])
	}
	return s
}

// maskTo keeps the start and the last four digits: "+94771234567" -> "+9477***4567".
// The table never holds a whole number or any text (FR-NTF-03).
func maskTo(phone string) string {
	if len(phone) < 9 {
		return "***"
	}
	return phone[:5] + "***" + phone[len(phone)-4:]
}

// record writes the row of one try. The row is its own transaction: it must
// stay when the caller's work is rolled back and tried again.
func (s *Service) record(ctx context.Context, m Message, o outcome) error {
	return s.db.Tx(ctx, db.System(), func(ctx context.Context, tx *db.Tx) error {
		return insertRow(ctx, tx, m, o)
	})
}

func insertRow(ctx context.Context, tx *db.Tx, m Message, o outcome) error {
	_, err := tx.Exec(ctx, `
		INSERT INTO notifications (shop_id, user_id, event_id, channel, template, to_masked, status, provider_ref, cost_units, error)
		VALUES ($1, $2, NULLIF($3, ''), $4, $5, $6, $7, NULLIF($8, ''), $9, NULLIF($10, ''))`,
		m.ShopID, m.UserID, m.EventID, m.Channel, m.Template, maskTo(m.To), o.status, cut(o.ref, maxRef), o.cost,
		cut(logx.RedactString(o.reason), maxReason))
	return err
}

// alreadySent is true when this event already reached this recipient with this
// template on this channel. Failed and skipped tries do not count: they may be repeated.
func (s *Service) alreadySent(ctx context.Context, m Message) (bool, error) {
	var done bool
	err := s.db.Tx(ctx, db.System(), func(ctx context.Context, tx *db.Tx) error {
		return tx.QueryRow(ctx, `
			SELECT EXISTS (SELECT 1 FROM notifications WHERE event_id = $1 AND channel = $2 AND template = $3 AND to_masked = $4
			                 AND status IN ('SENT', 'LINK'))`, m.EventID, m.Channel, m.Template, maskTo(m.To)).Scan(&done)
	})
	return done, err
}

// sentToday counts the SMS sent since midnight, Colombo time. The cap alerts are
// not counted: they report the cap and must always get through.
func sentToday(ctx context.Context, tx *db.Tx) (n int, err error) {
	err = tx.QueryRow(ctx, `SELECT count(*) FROM notifications WHERE channel = 'SMS' AND status = 'SENT' AND template NOT LIKE 'admin.cap%' AND created_at >= $1`,
		clock.StartOfDay(clock.Now())).Scan(&n)
	return n, err
}

// checkCap refuses an SMS once the day's cap is used. The refusal is recorded,
// and the admin hears of it once a day.
func (s *Service) checkCap(ctx context.Context, m Message) error {
	var n int
	if err := s.db.Tx(ctx, db.System(), func(ctx context.Context, tx *db.Tx) (err error) {
		n, err = sentToday(ctx, tx)
		return err
	}); err != nil {
		return err
	}
	if n < s.dailyCap {
		return nil
	}
	if err := s.record(ctx, m, outcome{status: statusSkipped, reason: "daily_cap"}); err != nil {
		return err
	}
	s.alert(ctx, AdminCap100, map[string]string{"cap": strconv.Itoa(s.dailyCap)})
	return ErrCapReached
}

// warnAtEighty tells the admin, once a day, that 80 % of the cap is used.
func (s *Service) warnAtEighty(ctx context.Context) {
	var n int
	err := s.db.Tx(ctx, db.System(), func(ctx context.Context, tx *db.Tx) (err error) {
		n, err = sentToday(ctx, tx)
		return err
	})
	if err != nil || s.dailyCap <= 0 || n*100 < s.dailyCap*80 {
		return
	}
	s.alert(ctx, AdminCap80, map[string]string{"percent": strconv.Itoa(n * 100 / s.dailyCap), "sent": strconv.Itoa(n), "cap": strconv.Itoa(s.dailyCap)})
}

// alert sends an admin text at most once a day per template. It skips the cap on
// purpose (the cap is what it reports), and a failure is logged and never stops
// the message that caused it. The lock holds a second alert back while the first
// is on its way.
func (s *Service) alert(ctx context.Context, template string, vars map[string]string) {
	ch, ok := s.channels[enum.NotifyChannelSms]
	if !ok || s.adminPhone == "" {
		return
	}
	vars["app"] = s.app
	m := Message{To: s.adminPhone, Template: template, Vars: vars, Channel: enum.NotifyChannelSms}
	text, err := Render(template, vars)
	if err != nil {
		logx.From(ctx).Warn("notify.alert_render", "template", template, "error", err.Error())
		return
	}
	m.Text = text
	err = s.db.Tx(ctx, db.System(), func(ctx context.Context, tx *db.Tx) error {
		if _, err := tx.Exec(ctx, `SELECT pg_advisory_xact_lock(hashtext($1))`, "notify.alert."+template); err != nil {
			return err
		}
		var sent bool
		if err := tx.QueryRow(ctx, `SELECT EXISTS (SELECT 1 FROM notifications WHERE template = $1 AND status = 'SENT' AND created_at >= $2)`,
			template, clock.StartOfDay(clock.Now())).Scan(&sent); err != nil || sent {
			return err
		}
		ref, cost, sendErr := ch.Send(ctx, m)
		o := outcome{status: statusSent, ref: ref, cost: cost}
		if sendErr != nil {
			o = outcome{status: statusFailed, reason: sendErr.Error()}
		}
		return insertRow(ctx, tx, m, o)
	})
	if err != nil {
		logx.From(ctx).Warn("notify.alert_failed", "template", template, "error", strings.ToValidUTF8(err.Error(), ""))
	}
}

package notify

import (
	"context"
	"net/http"
	"time"

	"github.com/google/uuid"

	"gp/internal/shared/apperr"
	"gp/internal/shared/clock"
	"gp/internal/shared/db"
	"gp/internal/shared/routex"
)

// maxUsageDays bounds one report.
const maxUsageDays = 93

// UsageRow is what was sent on one Colombo day on one channel with one result.
type UsageRow struct {
	Day       string `json:"day"`
	Channel   string `json:"channel"`
	Status    string `json:"status"`
	Count     int    `json:"count"`
	CostUnits int    `json:"costUnits"`
}

type Usage struct {
	From      string     `json:"from"`
	To        string     `json:"to"`
	DailyCap  int        `json:"dailyCap"`
	SentToday int        `json:"sentToday"`
	Data      []UsageRow `json:"data"`
}

func dateOf(field, s string, def time.Time) (time.Time, error) {
	if s == "" {
		return def, nil
	}
	t, err := clock.ParseDay(s)
	if err != nil {
		return t, apperr.Validation(map[string]string{field: "invalid"})
	}
	return t, nil
}

// Usage reports the sends between two Colombo dates, both days included; without
// dates it is the last 30 days. It reads only counts, never numbers or text.
func (s *Service) Usage(ctx context.Context, scope db.Scope, fromText, toText string) (Usage, error) {
	today := clock.StartOfDay(clock.Now())
	to, errTo := dateOf("to", toText, today)
	from, errFrom := dateOf("from", fromText, to.AddDate(0, 0, -29))
	if err := firstErr(errTo, errFrom); err != nil {
		return Usage{}, err
	}
	if from.After(to) || to.Sub(from) > maxUsageDays*24*time.Hour {
		return Usage{}, apperr.Validation(map[string]string{"from": "range"})
	}
	out := Usage{From: from.Format(time.DateOnly), To: to.Format(time.DateOnly), DailyCap: s.dailyCap, Data: []UsageRow{}}
	err := s.db.Tx(ctx, scope, func(ctx context.Context, tx *db.Tx) error {
		var err error
		if out.SentToday, err = sentToday(ctx, tx); err != nil {
			return err
		}
		rows, err := tx.Query(ctx, `
			SELECT (created_at AT TIME ZONE 'Asia/Colombo')::date::text AS day, channel, status, count(*), COALESCE(sum(cost_units), 0)::int
			FROM notifications WHERE created_at >= $1 AND created_at < $2
			GROUP BY 1, 2, 3 ORDER BY 1, 2, 3`, from, to.AddDate(0, 0, 1))
		if err != nil {
			return err
		}
		defer rows.Close()
		for rows.Next() {
			var r UsageRow
			if err := rows.Scan(&r.Day, &r.Channel, &r.Status, &r.Count, &r.CostUnits); err != nil {
				return err
			}
			out.Data = append(out.Data, r)
		}
		return rows.Err()
	})
	return out, err
}

func firstErr(errs ...error) error {
	for _, err := range errs {
		if err != nil {
			return err
		}
	}
	return nil
}

type Handler struct{ svc *Service }

func NewHandler(svc *Service) *Handler { return &Handler{svc: svc} }

// Usage handles GET /admin/notifications/usage?from=&to=.
func (h *Handler) Usage(w http.ResponseWriter, r *http.Request) {
	routex.Admin[struct{}](w, r, http.StatusOK, false, nil, func(r *http.Request, sc db.Scope, _ uuid.UUID) (any, error) {
		q := r.URL.Query()
		return h.svc.Usage(r.Context(), sc, q.Get("from"), q.Get("to"))
	})
}

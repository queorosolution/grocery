package notify

import (
	"fmt"
	"regexp"
	"strings"
)

// maxSMSLen keeps every text inside one SMS segment.
const maxSMSLen = 160

// Template names (docs/09 section 3.1).
const (
	OTP                 = "otp.phone"
	ShopNewBill         = "shop.new_bill"
	OrderApproved       = "order.approved"
	OrderRevised        = "order.revised"
	OrderRejected       = "order.rejected"
	OrderPaid           = "order.paid"
	OrderOut            = "order.out"
	OrderDelivered      = "order.delivered"
	OrderExpired        = "order.expired"
	OrderCancelled      = "order.cancelled"
	OrderDeliveryFailed = "order.delivery_failed"
	OrderRefundCancel   = "order.refund_cancel"
	AdminRefund         = "admin.refund"
	AdminConnector      = "admin.connector"
	AdminCap80          = "admin.cap80"
	AdminCap100         = "admin.cap100"
)

// templates are plain text with {name} slots; {app} is the platform name. The
// first group is the table in docs/09. The others cover the events of docs/04
// section 5 that the table has no text for. Every slot needs a limit in
// slotLimits, and TestTemplates_Max160 fills each slot to its limit.
var templates = map[string]string{
	OTP:                 "{app}: your code is {code}. Valid 10 min. Do not share it.",
	ShopNewBill:         "{app}: new bill {orderCode} ({items} items). Please review.",
	OrderApproved:       "{app}: {shop} confirmed order {orderCode}. Pay here: https://www.{domain}/order/{id}",
	OrderRevised:        "{app}: {shop} updated order {orderCode}. Please check: https://www.{domain}/order/{id}",
	OrderRejected:       "{app}: sorry, {shop} could not accept order {orderCode}.",
	OrderPaid:           "{app}: order {orderCode} is paid. Slot {slot}.",
	OrderOut:            "{app}: order {orderCode} is on the way. Give code {code} to the driver.",
	OrderDelivered:      "{app}: order {orderCode} delivered. Thank you!",
	OrderExpired:        "{app}: order {orderCode} expired.",
	OrderCancelled:      "{app}: order {orderCode} was cancelled.",
	OrderDeliveryFailed: "{app}: delivery of order {orderCode} failed. Please check and send it out again.",
	OrderRefundCancel:   "{app}: order {orderCode} was cancelled. The payment will be refunded.",
	AdminRefund:         "{app}: refund request for order {orderCode}, Rs {amount}. Please review.",
	AdminConnector:      "{app}: the connector of {shop} failed 3 times. Check the sync issues.",
	AdminCap80:          "{app}: SMS use is at {percent}% of today's cap ({sent} of {cap}).",
	AdminCap100:         "{app}: today's SMS cap ({cap}) is reached. New SMS are skipped until tomorrow.",
}

// slotLimits is the longest value each slot may hold, in characters. The values
// are chosen so the longest text still fits one SMS. A free-text value (a shop
// name) is cut at its limit; the two slots that build a link are never cut.
var slotLimits = map[string]int{
	"app": 16, "shop": 20, "orderCode": 12, "items": 3, "slot": 20, "code": 6, "amount": 12,
	"percent": 3, "sent": 7, "cap": 7, "domain": 20, "id": 36,
}

var (
	domainRe = regexp.MustCompile(`^[a-z0-9]([a-z0-9.-]*[a-z0-9])?$`)
	idRe     = regexp.MustCompile(`^[0-9a-f]{8}-[0-9a-f]{4}-[0-9a-f]{4}-[0-9a-f]{4}-[0-9a-f]{12}$`)
)

// Render fills a template. Values are cleaned (no control characters, no braces,
// cut at the slot's limit) so user text can never add a new line, a link or a
// slot. The link slots are not cleaned but checked: a domain or an id that is not
// exactly what it should be fails the render instead of making a wrong link.
func Render(name string, vars map[string]string) (string, error) {
	text, ok := templates[name]
	if !ok {
		return "", fmt.Errorf("notify: unknown template %q", name)
	}
	for _, slot := range slotsOf(text) {
		value, given := vars[slot]
		limit, known := slotLimits[slot]
		switch {
		case !known:
			return "", fmt.Errorf("notify: template %q has the slot %q with no limit", name, slot)
		case !given:
			return "", fmt.Errorf("notify: template %q has a slot without a value", name)
		}
		switch slot {
		case "domain":
			if len(value) > limit || !domainRe.MatchString(value) {
				return "", fmt.Errorf("notify: template %q: the domain is not usable in a link", name)
			}
		case "id":
			if !idRe.MatchString(value) {
				return "", fmt.Errorf("notify: template %q: the id is not an id", name)
			}
		default:
			value = clean(value, limit)
		}
		text = strings.ReplaceAll(text, "{"+slot+"}", value)
	}
	if len([]rune(text)) > maxSMSLen {
		return "", fmt.Errorf("notify: template %q is longer than %d characters", name, maxSMSLen)
	}
	return text, nil
}

// slotsOf lists the {slots} of a template text.
func slotsOf(text string) []string {
	var out []string
	for {
		i := strings.IndexByte(text, '{')
		if i < 0 {
			return out
		}
		j := strings.IndexByte(text[i:], '}')
		if j < 0 {
			return out
		}
		out = append(out, text[i+1:i+j])
		text = text[i+j+1:]
	}
}

func clean(s string, limit int) string {
	var b strings.Builder
	n := 0
	for _, r := range s {
		if n == limit {
			break
		}
		// ':', '/' and '@' go too, so a name can never become a link or an address.
		if r < ' ' || r == 0x7f || strings.ContainsRune("{}:/@", r) {
			continue
		}
		b.WriteRune(r)
		n++
	}
	return strings.TrimSpace(b.String())
}

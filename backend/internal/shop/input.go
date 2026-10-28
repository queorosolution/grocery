package shop

import (
	"regexp"
	"strings"
	"unicode/utf8"

	"gp/internal/shared/apperr"
	"gp/internal/shared/enum"
	"gp/internal/shared/validate"
)

var (
	slugRe  = regexp.MustCompile(`^[a-z0-9]+(-[a-z0-9]+)*$`)
	themeRe = regexp.MustCompile(`^#[0-9a-fA-F]{6}$`)
)

// Column limits from contracts/schema.sql.
const (
	minSlug, maxSlug = 3, 40
	minName, maxName = 2, 80
	maxAddress       = 200
	maxLogoURL       = 300
	defaultTheme     = "#222222"
)

// Sri Lanka's bounding box, the same as the table checks.
const (
	minLat, maxLat = 5.8, 10.0
	minLng, maxLng = 79.4, 82.0
)

// profile is a shop's editable fields after cleaning, ready to store.
type profile struct {
	Slug, Name, ThemeColor string
	Phone, Address, Logo   *string
	Lat, Lng               float64
}

func (p profile) validate() error {
	v := validate.New().
		Check("slug", slugRe.MatchString(p.Slug) && len(p.Slug) >= minSlug && len(p.Slug) <= maxSlug, "invalid").
		Str("name", p.Name, minName, maxName).
		Check("themeColor", themeRe.MatchString(p.ThemeColor), "invalid").
		Check("lat", p.Lat >= minLat && p.Lat <= maxLat, "out_of_range").
		Check("lng", p.Lng >= minLng && p.Lng <= maxLng, "out_of_range")
	if p.Phone != nil {
		v.Phone("phone", *p.Phone)
	}
	if p.Address != nil {
		v.Check("addressText", utf8.RuneCountInString(*p.Address) <= maxAddress, "too_long")
	}
	if p.Logo != nil {
		v.HTTPSURL("logoUrl", *p.Logo, maxLogoURL)
	}
	return v.Err()
}

// clean trims text, turns "" into "no value", and writes the phone as +94….
func (p *profile) clean() {
	p.Slug = strings.TrimSpace(p.Slug)
	p.Name = strings.TrimSpace(p.Name)
	p.Phone = blankToNil(p.Phone)
	p.Address = blankToNil(p.Address)
	p.Logo = blankToNil(p.Logo)
	if p.Phone != nil {
		if n, ok := validate.NormalizePhone(*p.Phone); ok {
			p.Phone = &n
		}
	}
}

func blankToNil(s *string) *string {
	if s == nil {
		return nil
	}
	t := strings.TrimSpace(*s)
	if t == "" {
		return nil
	}
	return &t
}

func profileFromCreate(in CreateInput) profile {
	p := profile{Slug: in.Slug, Name: in.Name, Phone: in.Phone, Address: in.AddressText, Logo: in.LogoURL,
		Lat: in.Lat, Lng: in.Lng, ThemeColor: defaultTheme}
	if in.ThemeColor != nil {
		p.ThemeColor = *in.ThemeColor
	}
	return p
}

// apply lays an update over the stored shop. An empty string clears the
// optional text fields; the other fields keep their value when absent.
func (p profile) apply(in UpdateInput) profile {
	if in.Slug != nil {
		p.Slug = *in.Slug
	}
	if in.Name != nil {
		p.Name = *in.Name
	}
	if in.Phone != nil {
		p.Phone = in.Phone
	}
	if in.AddressText != nil {
		p.Address = in.AddressText
	}
	if in.Lat != nil {
		p.Lat = *in.Lat
	}
	if in.Lng != nil {
		p.Lng = *in.Lng
	}
	if in.LogoURL != nil {
		p.Logo = in.LogoURL
	}
	if in.ThemeColor != nil {
		p.ThemeColor = *in.ThemeColor
	}
	return p
}

func profileOf(s Shop) profile {
	return profile{Slug: s.Slug, Name: s.Name, ThemeColor: s.ThemeColor, Phone: s.Phone, Address: s.AddressText,
		Logo: s.LogoURL, Lat: s.Lat, Lng: s.Lng}
}

// validateSettings checks every field against its enum or range (the table
// checks repeat these, but a clear field error beats a constraint failure).
func validateSettings(s Settings) error {
	v := validate.New().
		Check("deliveryType", s.DeliveryType.Valid(), "invalid").
		Check("paymentProvider", s.PaymentProvider.Valid(), "invalid").
		Check("driverSource", s.DriverSource.Valid(), "invalid").
		Check("notifyChannels", validChannels(s.NotifyChannels), "invalid").
		Int("markupBp", int64(s.MarkupBP), 0, 2000).
		Int("minPlatformMarginCents", s.MinPlatformMarginCents, 0, maxCents).
		Int("minOrderCents", s.MinOrderCents, 0, maxCents).
		Int("maxLines", int64(s.MaxLines), 1, 200).
		Int("reviewTimeoutMin", int64(s.ReviewTimeoutMin), 5, 120).
		Int("revisionTimeoutMin", int64(s.RevisionTimeoutMin), 5, 120).
		Int("paymentTimeoutMin", int64(s.PaymentTimeoutMin), 10, 240)
	return v.Err()
}

// maxCents keeps money in a sane range (Rs 1 billion) so sums cannot overflow.
const maxCents = 100_000_000_000

func validChannels(cs []enum.NotifyChannel) bool {
	if len(cs) == 0 {
		return false
	}
	seen := map[enum.NotifyChannel]bool{}
	for _, c := range cs {
		if !c.Valid() || seen[c] {
			return false
		}
		seen[c] = true
	}
	return true
}

// parseHours turns the request days into spans. Every day must be unique, in
// range, and close after it opens.
func parseHours(days []Day) (weekHours, error) {
	fields := map[string]string{}
	week := weekHours{}
	if len(days) > 7 {
		return nil, apperr.Validation(map[string]string{"days": "too_many"})
	}
	for _, d := range days {
		if d.Weekday < 1 || d.Weekday > 7 {
			fields["weekday"] = "invalid"
			continue
		}
		if _, dup := week[d.Weekday]; dup {
			fields["weekday"] = "duplicate"
			continue
		}
		opens, ok1 := parseClock(d.Opens, false)
		closes, ok2 := parseClock(d.Closes, true)
		switch {
		case !ok1:
			fields["opens"] = "invalid"
		case !ok2:
			fields["closes"] = "invalid"
		case closes <= opens:
			fields["closes"] = "before_opens"
		default:
			week[d.Weekday] = span{opens: opens, closes: closes}
		}
	}
	if len(fields) > 0 {
		return nil, apperr.Validation(fields)
	}
	return week, nil
}

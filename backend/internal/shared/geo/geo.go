// Package geo has the one distance formula used for delivery fees (D13): the
// straight line between two points. The road factor lives in the fee rule.
package geo

import "math"

type Point struct{ Lat, Lng float64 }

const earthRadiusM = 6371008.8 // mean radius, metres

// Sri Lanka's bounding box. It matches the CHECK constraints of the shop and
// address tables, so a pin that passes here is never refused by the database.
const (
	MinLat, MaxLat = 5.8, 10.0
	MinLng, MaxLng = 79.4, 82.0
)

// InSriLanka reports whether the point is inside the box above (FR-ADDR-04).
func (p Point) InSriLanka() bool {
	return p.Lat >= MinLat && p.Lat <= MaxLat && p.Lng >= MinLng && p.Lng <= MaxLng
}

// Valid reports whether the point is a real place on Earth.
func (p Point) Valid() bool {
	return p.Lat >= -90 && p.Lat <= 90 && p.Lng >= -180 && p.Lng <= 180
}

// HaversineM returns the straight-line distance in whole metres.
func HaversineM(a, b Point) int {
	rad := math.Pi / 180
	dLat := (b.Lat - a.Lat) * rad
	dLng := (b.Lng - a.Lng) * rad
	h := math.Sin(dLat/2)*math.Sin(dLat/2) +
		math.Cos(a.Lat*rad)*math.Cos(b.Lat*rad)*math.Sin(dLng/2)*math.Sin(dLng/2)
	return int(math.Round(2 * earthRadiusM * math.Asin(math.Min(1, math.Sqrt(h)))))
}

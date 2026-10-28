package geo

import "testing"

func TestHaversineM(t *testing.T) {
	colombo := Point{6.9271, 79.8612}
	kandy := Point{7.2906, 80.6337}
	cases := []struct {
		name     string
		a, b     Point
		min, max int
	}{
		{"same point", colombo, colombo, 0, 0},
		{"Colombo to Kandy is about 94 km straight (road is longer, hence the road factor)", colombo, kandy, 93_500, 95_000},
		{"one degree of latitude is about 111 km", Point{0, 0}, Point{1, 0}, 111_000, 111_400},
		{"across the date line", Point{0, 179.5}, Point{0, -179.5}, 111_000, 111_400},
	}
	for _, tc := range cases {
		got := HaversineM(tc.a, tc.b)
		if got < tc.min || got > tc.max {
			t.Errorf("%s: got %d m, want %d to %d", tc.name, got, tc.min, tc.max)
		}
		if back := HaversineM(tc.b, tc.a); back != got {
			t.Errorf("%s: not symmetric (%d vs %d)", tc.name, got, back)
		}
	}
}

func TestValid(t *testing.T) {
	if !(Point{6.9, 79.8}).Valid() || (Point{91, 0}).Valid() || (Point{0, 181}).Valid() {
		t.Fatal("Valid gives the wrong answer")
	}
}

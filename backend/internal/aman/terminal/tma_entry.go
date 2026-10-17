package terminal

import (
	"math"
	"sort"
)

// EntryFraction returns the first intersection of a predicted linear leg with
// the validated 3D volume, including descent through the strict FL195 ceiling.
func (v TMAVolume) EntryFraction(lat1, lon1, alt1, lat2, lon2, alt2 float64) (float64, bool) {
	for _, value := range []float64{lat1, lon1, alt1, lat2, lon2, alt2} {
		if math.IsNaN(value) || math.IsInf(value, 0) {
			return 0, false
		}
	}
	fractions := []float64{0, 1}
	dx, dy := lon2-lon1, lat2-lat1
	for _, polygon := range v.polygons {
		for _, ring := range polygon {
			for i := 1; i < len(ring); i++ {
				a, b := ring[i-1], ring[i]
				ex, ey := b.longitude-a.longitude, b.latitude-a.latitude
				denom := dx*ey - dy*ex
				if math.Abs(denom) < 1e-14 {
					continue
				}
				ax, ay := a.longitude-lon1, a.latitude-lat1
				t, u := (ax*ey-ay*ex)/denom, (ax*dy-ay*dx)/denom
				if t >= 0 && t <= 1 && u >= 0 && u <= 1 {
					fractions = append(fractions, t)
				}
			}
		}
	}
	if alt2 != alt1 {
		for _, alt := range []float64{0, EKCHTMAUpperAltitudeFeet} {
			t := (alt - alt1) / (alt2 - alt1)
			if t > 0 && t < 1 {
				fractions = append(fractions, t)
			}
		}
	}
	sort.Float64s(fractions)
	contains := func(t float64) bool { return v.Contains(lat1+dy*t, lon1+dx*t, alt1+(alt2-alt1)*t) }
	for i, t := range fractions {
		if contains(t) {
			return t, true
		}
		if i+1 < len(fractions) && contains((t+fractions[i+1])/2) {
			// The horizontal boundary is included, but FL195 itself is outside.
			return math.Nextafter(t, 1), true
		}
	}
	return 0, false
}

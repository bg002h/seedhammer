package slip39

// Lagrange interpolation over GF(256). Port of mnemonic_toolkit::slip39::lagrange.

const (
	secretIndex = 255 // SLIP-0039 SECRET_INDEX
	digestIndex = 254 // SLIP-0039 DIGEST_INDEX
	digestLen   = 4
)

type point struct{ x, y byte }
type bytePoint struct {
	x byte
	y []byte
}

// interpolateAt evaluates the degree-(len-1) polynomial through points at x.
// Precondition: all points[i].x distinct (enforced upstream; see SPEC §4.4).
func interpolateAt(points []point, x byte) byte {
	var result byte
	for i := range points {
		xi, yi := points[i].x, points[i].y
		num, den := byte(1), byte(1)
		for j := range points {
			if i == j {
				continue
			}
			xj := points[j].x
			num = gfMul(num, gfAdd(x, xj))
			den = gfMul(den, gfAdd(xi, xj))
		}
		result = gfAdd(result, gfMul(yi, gfDiv(num, den)))
	}
	return result
}

// interpolateSecretAt interpolates each byte position independently.
// All points[i].y must be equal length (enforced upstream).
func interpolateSecretAt(points []bytePoint, x byte) []byte {
	n := len(points[0].y)
	out := make([]byte, n)
	pb := make([]point, len(points))
	for k := 0; k < n; k++ {
		for i := range points {
			pb[i] = point{points[i].x, points[i].y[k]}
		}
		out[k] = interpolateAt(pb, x)
	}
	return out
}

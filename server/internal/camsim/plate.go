package camsim

import (
	"math/rand/v2"
	"strings"

	"github.com/EziosWJ/weighbridge/server/internal/weighing"
)

const plateProvince = "晋京津冀蒙辽吉黑沪苏浙皖闽赣鲁豫鄂湘粤桂琼渝川贵云藏陕甘青宁新"

var plateProvinces = []rune(plateProvince)

// Excludes I and O, which mainland plates do not use.
const plateLetters = "ABCDEFGHJKLMNPQRSTUVWXYZ"
const plateChars = plateLetters + "0123456789"
const plateDigits = "0123456789"

// newPlate builds a canonical plate that satisfies the business validation
// rule. Ordinary plates are more common, with new-energy formats mixed in so
// both shapes can be exercised during development.
func newPlate() string {
	var b strings.Builder
	b.WriteRune(plateProvinces[rand.IntN(len(plateProvinces))])
	b.WriteByte(plateLetters[rand.IntN(len(plateLetters))])
	switch rand.IntN(10) {
	case 0, 1:
		b.WriteByte("DF"[rand.IntN(2)])
		b.WriteByte(plateChars[rand.IntN(len(plateChars))])
		writeRandom(&b, plateDigits, 4)
	case 2, 3:
		writeRandom(&b, plateDigits, 5)
		b.WriteByte("DF"[rand.IntN(2)])
	default:
		writeRandom(&b, plateChars, 5)
	}
	plate := b.String()
	if !weighing.IsBusinessPlate(plate) {
		// Defensive fallback; the construction above is already valid.
		return "晋A12345"
	}
	return plate
}

func writeRandom(b *strings.Builder, source string, count int) {
	for i := 0; i < count; i++ {
		b.WriteByte(source[rand.IntN(len(source))])
	}
}

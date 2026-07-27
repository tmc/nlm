package richrender

import "testing"

func TestUTF16Len(t *testing.T) {
	cases := []struct {
		in   string
		want int
	}{
		{"", 0},
		{"abc", 3},
		{"héllo", 5},         // é is BMP: 1 unit
		{"a📊b", 4},           // 📊 is astral: 2 units
		{"📊📊", 4},            // two astral: 4 units
		{"aé\U0001F4CAz", 5}, // a, é, 📊, z → 1+1+2+1
	}
	for _, tc := range cases {
		if got := utf16Len(tc.in); got != tc.want {
			t.Errorf("utf16Len(%q) = %d, want %d", tc.in, got, tc.want)
		}
	}
}

// TestUTF16RuneMap checks that a wire (UTF-16) offset maps to the rune index at
// that boundary, and that the astral case shifts by exactly one per surrogate.
func TestUTF16RuneMap(t *testing.T) {
	// "📊 Alpha" — 📊 = runes[0] = UTF-16 units 0..2; space = unit 2 = rune 1;
	// 'A' = unit 3 = rune 2.
	content := "📊 Alpha"
	m := newUTF16RuneMap(content)
	cases := []struct {
		u16  int
		want int // rune index
	}{
		{0, 0},  // start of 📊
		{2, 1},  // the space (rune 1) — 📊 consumed 2 units
		{3, 2},  // 'A' (rune 2)
		{8, 7},  // end: "📊 Alpha" is 7 runes, UTF-16 len 8
		{99, 7}, // clamp past end
		{-1, 0}, // clamp before start
	}
	for _, tc := range cases {
		if got := m.rune(tc.u16); got != tc.want {
			t.Errorf("map.rune(%d) = %d, want %d (content=%q)", tc.u16, got, tc.want, content)
		}
	}
}

// TestUTF16RuneMapIdentityBMP verifies the map is the identity for all-BMP text,
// so the common case is a no-op.
func TestUTF16RuneMapIdentityBMP(t *testing.T) {
	content := "Plain héllo, wörld."
	m := newUTF16RuneMap(content)
	runes := []rune(content)
	for i := 0; i <= len(runes); i++ {
		if got := m.rune(i); got != i {
			t.Errorf("BMP map.rune(%d) = %d, want identity %d", i, got, i)
		}
	}
}

func TestByteToUTF16(t *testing.T) {
	// "📊[1]" — 📊 is 4 bytes / 2 UTF-16 units; '[' starts at byte 4 = unit 2.
	content := "📊[1]"
	m := byteToUTF16(content)
	if got := m[0]; got != 0 {
		t.Errorf("byteToUTF16[0] = %d, want 0", got)
	}
	if got := m[4]; got != 2 { // '[' after the emoji
		t.Errorf("byteToUTF16[4] = %d, want 2 (emoji is 2 UTF-16 units)", got)
	}
	if got := m[len(content)]; got != 5 { // 📊(2) + [(1) + 1(1) + ](1)
		t.Errorf("byteToUTF16[len] = %d, want 5", got)
	}
}

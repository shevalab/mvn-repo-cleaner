package resolve

import "testing"

func TestCompareVersions(t *testing.T) {
	cases := []struct {
		a, b string
		want int
	}{
		{"1.0", "1.0", 0},
		{"1.0", "1.1", -1},
		{"1.10", "1.9", 1},
		{"2.0", "1.99", 1},
		{"1.0.1", "1.0", 1},
		{"1.0-SNAPSHOT", "1.0", -1},
		{"2.0", "1.0", 1},
	}
	for _, c := range cases {
		got := compareVersions(c.a, c.b)
		if (got < 0) != (c.want < 0) || (got > 0) != (c.want > 0) {
			t.Errorf("compareVersions(%q,%q)=%d, want sign %d", c.a, c.b, got, c.want)
		}
	}
}

func TestParseRange(t *testing.T) {
	lo, hi, loIn, hiIn := parseRange("[1.0,2.0)")
	if lo != "1.0" || hi != "2.0" || !loIn || hiIn {
		t.Fatalf("got lo=%q hi=%q loIn=%v hiIn=%v", lo, hi, loIn, hiIn)
	}
	lo, hi, loIn, hiIn = parseRange("(,2.0]")
	if lo != "" || hi != "2.0" || loIn || !hiIn {
		t.Fatalf("got lo=%q hi=%q loIn=%v hiIn=%v", lo, hi, loIn, hiIn)
	}
}

func TestInterp(t *testing.T) {
	props := map[string]string{"v": "1.2.3"}
	if got := interp("${v}", props); got != "1.2.3" {
		t.Fatalf("got %q", got)
	}
	if got := interp("x-${missing}-y", props); got != "x-${missing}-y" {
		t.Fatalf("unresolved token changed: %q", got)
	}
}

func TestInRange(t *testing.T) {
	if !inRange("1.5", "1.0", "2.0", true, true) {
		t.Fatal("1.5 should be in [1.0,2.0]")
	}
	if inRange("2.0", "1.0", "2.0", true, false) {
		t.Fatal("2.0 should not be in [1.0,2.0)")
	}
	if inRange("0.9", "1.0", "2.0", true, true) {
		t.Fatal("0.9 should not be in [1.0,2.0]")
	}
}

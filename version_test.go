package versioning

import (
	"math"
	"testing"
	"time"
)

func TestCoreParsing(t *testing.T) {
	for _, s := range []string{"0.0.0", "1.2.3", "18446744073709551615.0.9"} {
		c, err := ParseCore(s)
		if err != nil || c.String() != s {
			t.Errorf("ParseCore(%q) = %v, %v", s, c, err)
		}
	}
	for _, s := range []string{"", "1.2", "1.2.3.4", "v1.2.3", "01.2.3", "1.-2.3", "+1.2.3", " 1.2.3", "1.2.3\n", "18446744073709551616.0.0"} {
		if _, err := ParseCore(s); err == nil {
			t.Errorf("accepted invalid core %q", s)
		}
	}
}

func TestCoreBump(t *testing.T) {
	for _, tc := range []struct {
		b    Bump
		want string
	}{{BumpInherit, "1.2.3"}, {"", "1.2.3"}, {BumpMajor, "2.0.0"}, {BumpMinor, "1.3.0"}, {BumpPatch, "1.2.4"}} {
		got, err := (Core{1, 2, 3}).Bump(tc.b)
		if err != nil || got.String() != tc.want {
			t.Errorf("bump %q = %v, %v", tc.b, got, err)
		}
	}
	for _, tc := range []struct {
		c Core
		b Bump
	}{{Core{math.MaxUint64, 2, 3}, BumpMajor}, {Core{1, math.MaxUint64, 3}, BumpMinor}, {Core{1, 2, math.MaxUint64}, BumpPatch}, {Core{}, "invalid"}} {
		if _, err := tc.c.Bump(tc.b); err == nil {
			t.Errorf("accepted invalid bump %+v", tc)
		}
	}
}

func TestApplicationVersionParsing(t *testing.T) {
	for _, s := range []string{"1.2.3+20240229.7", "0.0.0-snapshot+20260101.18446744073709551615"} {
		got, err := Parse(s)
		if err != nil || got.String() != s {
			t.Errorf("Parse(%q) = %v, %v", s, got, err)
		}
	}
	for _, s := range []string{"1.2.3", "v1.2.3+20260101.7", "1.2.3+20260229.7", "1.2.3+20260431.7", "1.2.3+20260001.7", "1.2.3+00000101.7", "1.2.3+20260101.0", "1.2.3+20260101.07", "1.2.3+20260101.-1", "1.2.3+20260101.18446744073709551616", "1.2.3-other+20260101.7", "1.2.3+20260101.7\n", UnknownVersion} {
		if _, err := Parse(s); err == nil {
			t.Errorf("accepted invalid application version %q", s)
		}
	}
}

func TestTagsPreserveCanonicalAndLegacyIdentity(t *testing.T) {
	for _, tc := range []struct {
		input  string
		legacy bool
	}{{"v1.2.3+20260101.7", false}, {"build/1.2.3+20260101.7", true}} {
		v, legacy, err := ParseTag(tc.input)
		if err != nil || legacy != tc.legacy || v.String() != "1.2.3+20260101.7" {
			t.Errorf("ParseTag(%q)=%v,%v,%v", tc.input, v, legacy, err)
		}
		tag, err := v.Tag()
		if err != nil || tag != "v1.2.3+20260101.7" {
			t.Errorf("Tag=%q,%v", tag, err)
		}
	}
	for _, s := range []string{"1.2.3+20260101.7", "v1.2.3", "build/v1.2.3+20260101.7", "v1.2.3-snapshot+20260101.7", "build/1.2.3-snapshot+20260101.7"} {
		if _, _, err := ParseTag(s); err == nil {
			t.Errorf("accepted tag %q", s)
		}
	}
	if _, err := (Version{}).Tag(); err == nil {
		t.Error("accepted zero version tag")
	}
	v, _ := Parse("1.2.3-snapshot+20260101.7")
	if _, err := v.Tag(); err == nil {
		t.Error("accepted snapshot tag")
	}
}

func TestNextUsesSaoPauloCommitDate(t *testing.T) {
	commitTime := time.Date(2026, 1, 2, 1, 30, 0, 0, time.UTC)
	v, err := Next(Core{1, 2, 3}, commitTime, 42, BumpMinor)
	if err != nil || v.String() != "1.3.0+20260101.42" {
		t.Fatalf("Next=%v,%v", v, err)
	}
	if _, err := Next(Core{}, time.Time{}, 1, ""); err == nil {
		t.Error("accepted zero commit time")
	}
	if _, err := Next(Core{}, commitTime, 0, ""); err == nil {
		t.Error("accepted zero PR")
	}
	if _, err := Next(Core{}, time.Date(10000, 1, 2, 0, 0, 0, 0, time.UTC), 1, ""); err == nil {
		t.Error("accepted unrepresentable date")
	}
}

func TestNextHonorsHistoricalSaoPauloDaylightSaving(t *testing.T) {
	v, err := Next(Core{1, 0, 0}, time.Date(2018, 11, 5, 2, 30, 0, 0, time.UTC), 1, "")
	if err != nil || v.String() != "1.0.0+20181105.1" {
		t.Fatalf("historical local date=%v,%v", v, err)
	}
}

func TestNextPreservesCalendarDateAcrossSaoPauloTransitions(t *testing.T) {
	for _, tc := range []struct{ instant, want string }{
		{"2018-11-04T02:59:59Z", "1.0.0+20181103.1"},
		{"2018-11-04T03:00:00Z", "1.0.0+20181104.1"},
		{"2018-11-04T12:00:00Z", "1.0.0+20181104.1"},
		{"2018-11-05T01:59:59Z", "1.0.0+20181104.1"},
		{"2018-11-05T02:00:00Z", "1.0.0+20181105.1"},
		{"2019-02-17T01:59:59Z", "1.0.0+20190216.1"},
		{"2019-02-17T02:00:00Z", "1.0.0+20190216.1"},
		{"2019-02-17T02:59:59Z", "1.0.0+20190216.1"},
		{"2019-02-17T03:00:00Z", "1.0.0+20190217.1"},
	} {
		t.Run(tc.instant, func(t *testing.T) {
			instant, err := time.Parse(time.RFC3339, tc.instant)
			if err != nil {
				t.Fatal(err)
			}
			got, err := Next(Core{1, 0, 0}, instant, 1, "")
			if err != nil {
				t.Fatal(err)
			}
			if got.String() != tc.want {
				t.Errorf("Next date=%s, want %s", got.String(), tc.want)
			}
			if got.Date.Location() != time.UTC || got.Date.Hour() != 0 || got.Date.Minute() != 0 || got.Date.Second() != 0 || got.Date.Nanosecond() != 0 {
				t.Errorf("Date must be a UTC calendar-date value, got %v", got.Date)
			}
		})
	}
}

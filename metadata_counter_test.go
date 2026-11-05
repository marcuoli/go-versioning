package versioning

import (
	"encoding/json"
	"math"
	"strconv"
	"testing"
)

func TestCounterLinkerValuesAndUnknownCompatibility(t *testing.T) {
	for _, value := range []string{"", "0", "1", strconv.FormatUint(math.MaxUint64, 10)} {
		if _, err := ParseBuildNumber(value); err != nil {
			t.Fatalf("valid number %q: %v", value, err)
		}
	}
	for _, value := range []string{"00", "01", "-1", "+1", " 1", "1.0", "18446744073709551616"} {
		if _, err := ParseBuildNumber(value); err == nil {
			t.Errorf("accepted %q", value)
		}
	}
	info, err := Current(Info{})
	if err != nil || info.OverallBuildNumber != 0 || info.BuildCounterID != "" {
		t.Fatalf("unknown counter: %+v %v", info, err)
	}
	data, _ := json.Marshal(Info{Version: UnknownVersion})
	if string(data) != `{"version":"0.0.0+unknown","commit":"","build_time":""}` {
		t.Fatalf("changed unknown JSON: %s", data)
	}
}

func TestCounterMetadataCannotBecomeAnUnboundHybrid(t *testing.T) {
	full := Info{Version: "1.2.3+20260101.7", Commit: commitA, BuildTime: "2026-01-01T12:00:00Z", OverallBuildNumber: 3, BuildCounterID: "0123456789abcdef0123456789abcdef"}
	got, err := Current(full)
	if err != nil || got != full {
		t.Fatalf("complete injection: %+v %v", got, err)
	}
	for _, bad := range []Info{
		{OverallBuildNumber: 1}, {BuildCounterID: full.BuildCounterID},
		{Version: full.Version, OverallBuildNumber: 1, BuildCounterID: full.BuildCounterID},
		{Version: full.Version, Commit: full.Commit, BuildTime: full.BuildTime, OverallBuildNumber: 1, BuildCounterID: "invalid"},
	} {
		if _, err := Resolve(bad, Info{}); err == nil {
			t.Errorf("accepted unbound counter %+v", bad)
		}
	}
	got, err = Resolve(Info{Commit: commitA}, full)
	if err != nil || got != full {
		t.Fatalf("matching embedded counter: %+v %v", got, err)
	}
	if _, err := Resolve(Info{BuildTime: "2026-01-02T12:00:00Z"}, full); err == nil {
		t.Fatal("mixed a counter with different build time")
	}
	if _, err := Resolve(Info{Version: UnknownVersion}, full); err == nil {
		t.Fatal("mixed a counter with a different version")
	}
}

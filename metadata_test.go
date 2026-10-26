package versioning

import (
	"encoding/json"
	"runtime/debug"
	"strings"
	"testing"
)

const commitA = "aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa"
const commitB = "bbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbb"

func TestResolveMetadataPrecedence(t *testing.T) {
	full := Info{Version: "1.2.3+20260101.7", Commit: commitA, BuildTime: "2026-01-01T12:00:00Z"}
	got, err := Resolve(full, Info{Version: "invalid", Commit: "invalid", BuildTime: "invalid"})
	if err != nil || got != full {
		t.Fatalf("complete injection=%+v,%v", got, err)
	}
	got, err = Resolve(Info{Version: full.Version, Commit: commitA}, Info{Version: "1.2.2+20251231.6", Commit: commitA, BuildTime: full.BuildTime})
	if err != nil || got != full {
		t.Fatalf("partial injection=%+v,%v", got, err)
	}
	if _, err := Resolve(Info{Version: full.Version, Commit: commitA}, Info{Commit: commitB, BuildTime: full.BuildTime}); err == nil {
		t.Error("mixed conflicting commits")
	}
	got, err = Resolve(Info{}, Info{})
	if err != nil || got.Version != UnknownVersion || got.Commit != "" || got.BuildTime != "" {
		t.Fatalf("unknown=%+v,%v", got, err)
	}
	got, err = Resolve(Info{Version: UnknownVersion}, Info{Version: full.Version, Commit: commitA, BuildTime: full.BuildTime})
	if err != nil || got.Version != UnknownVersion {
		t.Fatalf("explicit unknown=%+v,%v", got, err)
	}
	for _, s := range []string{"v0.1.0", "v0.1.0-rc.1", "(devel)"} {
		got, err = Resolve(Info{}, Info{Version: s})
		if err != nil || got.Version != UnknownVersion {
			t.Errorf("embedded module %q=%+v,%v", s, got, err)
		}
	}
}

func TestResolveRejectsMalformedFields(t *testing.T) {
	for _, bad := range []Info{{Version: "1.2.3"}, {Version: "garbage"}, {Commit: "abcdef0"}, {Commit: strings.Repeat("g", 40)}, {Commit: strings.Repeat("a", 41)}, {BuildTime: "2026-02-29T00:00:00Z"}, {BuildTime: "2026-01-01"}} {
		if _, err := Resolve(bad, Info{}); err == nil {
			t.Errorf("accepted injected %+v", bad)
		}
	}
	got, err := Resolve(Info{Commit: strings.Repeat("A", 64), BuildTime: "2026-01-01T12:00:00-03:00"}, Info{})
	if err != nil || got.Commit != strings.Repeat("a", 64) {
		t.Fatalf("full SHA256=%+v,%v", got, err)
	}
	data, err := json.Marshal(Info{Version: UnknownVersion, Commit: commitA, BuildTime: "2026-01-01T12:00:00Z"})
	if err != nil || string(data) != `{"version":"0.0.0+unknown","commit":"aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa","build_time":"2026-01-01T12:00:00Z"}` {
		t.Fatalf("JSON=%s,%v", data, err)
	}
}

func TestEmbeddedMetadataUsesMainExecutable(t *testing.T) {
	bi := &debug.BuildInfo{Main: debug.Module{Path: "example.com/application", Version: "v1.2.3+20260101.7"}, Deps: []*debug.Module{{Path: "github.com/marcuoli/go-versioning", Version: "v9.9.9"}}, Settings: []debug.BuildSetting{{Key: "vcs.revision", Value: commitA}, {Key: "vcs.time", Value: "2026-01-01T12:00:00Z"}, {Key: "vcs.modified", Value: "true"}}}
	got := infoFromBuildInfo(bi)
	if got.Version != "1.2.3-snapshot+20260101.7" || got.Commit != commitA || got.BuildTime != "2026-01-01T12:00:00Z" {
		t.Fatalf("embedded=%+v", got)
	}
	bi.Main.Version = "v0.1.0"
	if got := infoFromBuildInfo(bi); got.Version != UnknownVersion {
		t.Fatalf("dependency/module became app identity: %+v", got)
	}
	if got := infoFromBuildInfo(nil); got.Version != UnknownVersion {
		t.Fatalf("missing build info=%+v", got)
	}
}

func TestCurrentKeepsCompleteInjection(t *testing.T) {
	in := Info{Version: "1.2.3+20260101.7", Commit: commitA, BuildTime: "2026-01-01T12:00:00Z"}
	got, err := Current(in)
	if err != nil || got != in {
		t.Fatalf("Current=%+v,%v", got, err)
	}
	got = ReadBuildInfo()
	if got.Version == "" {
		t.Fatal("ReadBuildInfo returned empty version")
	}
}

func TestResolveRequiresRFC3339TimeSyntax(t *testing.T) {
	for _, s := range []string{"2026-01-01T12:00:00+24:00", "2026-01-01T12:00:00+00:60", "2026-01-01T1:00:00Z", "2026-01-01T12:00:00,1Z"} {
		if _, err := Resolve(Info{BuildTime: s}, Info{}); err == nil {
			t.Errorf("accepted non-RFC3339 time %q", s)
		}
	}
}

package versioning

import (
	"strings"
	"testing"
)

func TestSharedLinkerContractAndUnknownRuntime(t *testing.T) {
	got, err := CurrentBuild()
	if err != nil || got.OverallBuildNumber != 0 {
		t.Fatalf("default=%+v %v", got, err)
	}
	complete := Info{Version: UnknownVersion, Commit: commitA, BuildTime: "2026-01-01T12:00:00Z", OverallBuildNumber: 4, BuildCounterID: "0123456789abcdef0123456789abcdef"}
	flags, err := complete.LinkerFlags()
	if err != nil || !strings.Contains(flags, "injectedBuildNumber=4") || !strings.Contains(flags, "injectedCounterID="+complete.BuildCounterID) {
		t.Fatalf("flags=%s %v", flags, err)
	}
	if _, err := (Info{}).LinkerFlags(); err == nil {
		t.Fatal("accepted incomplete build metadata")
	}
	if _, err := (Info{Version: "1.2.3 bad", Commit: commitA, BuildTime: complete.BuildTime}).LinkerFlags(); err == nil {
		t.Fatal("accepted unsafe metadata")
	}
}

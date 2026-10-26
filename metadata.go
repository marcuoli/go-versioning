package versioning

import (
	"fmt"
	"regexp"
	"runtime/debug"
	"strings"
	"time"
)

// Info describes an application build. Empty Commit and BuildTime mean unknown;
// Commit, when present, is a full 40- or 64-digit hexadecimal identity, and
// BuildTime, when present, is RFC3339. Version uses UnknownVersion when unknown.
type Info struct {
	Version   string `json:"version"`
	Commit    string `json:"commit"`
	BuildTime string `json:"build_time"`
}

var buildTimePattern = regexp.MustCompile(`^[0-9]{4}-[0-9]{2}-[0-9]{2}T(?:[01][0-9]|2[0-3]):[0-5][0-9]:[0-5][0-9](?:\.[0-9]+)?(?:Z|[+-](?:[01][0-9]|2[0-3]):[0-5][0-9])$`)

var commitPattern = regexp.MustCompile(`^(?:[0-9a-fA-F]{40}|[0-9a-fA-F]{64})$`)
var moduleVersionPattern = regexp.MustCompile(`^v(?:0|[1-9][0-9]*)\.(?:0|[1-9][0-9]*)\.(?:0|[1-9][0-9]*)(?:-[0-9A-Za-z-]+(?:\.[0-9A-Za-z-]+)*)?(?:\+[0-9A-Za-z-]+(?:\.[0-9A-Za-z-]+)*)?$`)

// Resolve validates and combines injected and embedded application metadata.
// Complete injection wins without inspecting embedded fields. Partial injection
// takes precedence field by field, but conflicting full commit identities are
// rejected. Ordinary embedded Go module versions are treated as absent.
// An explicit injected UnknownVersion is preserved rather than filled in.
func Resolve(injected, embedded Info) (Info, error) {
	in, err := validateInfo(injected, false)
	if err != nil {
		return Info{}, err
	}
	if complete(in) {
		return in, nil
	}
	em, err := validateInfo(embedded, true)
	if err != nil {
		return Info{}, err
	}
	if in.Commit != "" && em.Commit != "" && in.Commit != em.Commit {
		return Info{}, fmt.Errorf("injected and embedded commit identities conflict")
	}
	if in.Version == "" {
		in.Version = em.Version
	}
	if in.Commit == "" {
		in.Commit = em.Commit
	}
	if in.BuildTime == "" {
		in.BuildTime = em.BuildTime
	}
	if in.Version == "" {
		in.Version = UnknownVersion
	}
	return in, nil
}

func complete(info Info) bool { return info.Version != "" && info.Commit != "" && info.BuildTime != "" }

func validateInfo(info Info, embedded bool) (Info, error) {
	if info.Version != "" && info.Version != UnknownVersion {
		value := info.Version
		if embedded && strings.HasPrefix(value, "v") {
			value = strings.TrimPrefix(value, "v")
		}
		parsed, err := Parse(value)
		if err != nil {
			if embedded && (info.Version == "(devel)" || moduleVersionPattern.MatchString(info.Version)) {
				info.Version = ""
			} else {
				return Info{}, fmt.Errorf("invalid build version: %w", err)
			}
		} else {
			info.Version = parsed.String()
		}
	}
	if info.Commit != "" {
		if !commitPattern.MatchString(info.Commit) {
			return Info{}, fmt.Errorf("build commit must be a full 40- or 64-digit hexadecimal identity")
		}
		info.Commit = strings.ToLower(info.Commit)
	}
	if info.BuildTime != "" {
		if !buildTimePattern.MatchString(info.BuildTime) {
			return Info{}, fmt.Errorf("build time must use RFC3339 syntax")
		}
		if _, err := time.Parse(time.RFC3339, info.BuildTime); err != nil {
			return Info{}, fmt.Errorf("invalid build time: %w", err)
		}
	}
	return info, nil
}

// ReadBuildInfo reads metadata from the main executable's embedded Go build
// information. It never uses dependency module versions or probes a repository.
// Ordinary module versions and unavailable application versions become unknown.
// The VCS revision and time, when present, are returned for Resolve to validate.
func ReadBuildInfo() Info {
	build, ok := debug.ReadBuildInfo()
	if !ok {
		return Info{Version: UnknownVersion}
	}
	return infoFromBuildInfo(build)
}

func infoFromBuildInfo(build *debug.BuildInfo) Info {
	info := Info{Version: UnknownVersion}
	if build == nil {
		return info
	}
	v, err := Parse(strings.TrimPrefix(build.Main.Version, "v"))
	modified := false
	for _, setting := range build.Settings {
		switch setting.Key {
		case "vcs.revision":
			info.Commit = setting.Value
		case "vcs.time":
			info.BuildTime = setting.Value
		case "vcs.modified":
			modified = setting.Value == "true"
		}
	}
	if err == nil {
		v.Snapshot = v.Snapshot || modified
		info.Version = v.String()
	}
	return info
}

// Current resolves injection against the main executable's build information.
// Complete valid injection bypasses runtime build-information discovery.
func Current(injected Info) (Info, error) {
	info, err := validateInfo(injected, false)
	if err != nil {
		return Info{}, err
	}
	if complete(info) {
		return info, nil
	}
	return Resolve(info, ReadBuildInfo())
}

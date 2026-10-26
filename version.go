package versioning

import (
	"fmt"
	"math"
	"regexp"
	"strconv"
	"strings"
	"time"
	_ "time/tzdata" // Keep the allocation timezone available on minimal systems.
)

// UnknownVersion identifies a build with no known application version.
const UnknownVersion = "0.0.0+unknown"

// Core is the numeric SemVer core shared by successive application builds.
type Core struct {
	Major uint64
	Minor uint64
	Patch uint64
}

// Bump selects the numeric component to advance before allocating a version.
type Bump string

const (
	// BumpInherit keeps the previous core. The empty Bump has the same meaning.
	BumpInherit Bump = "inherit"
	// BumpMajor increments Major and resets Minor and Patch.
	BumpMajor Bump = "major"
	// BumpMinor increments Minor and resets Patch.
	BumpMinor Bump = "minor"
	// BumpPatch increments Patch.
	BumpPatch Bump = "patch"
)

var corePattern = regexp.MustCompile(`^(0|[1-9][0-9]*)\.(0|[1-9][0-9]*)\.(0|[1-9][0-9]*)$`)
var versionPattern = regexp.MustCompile(`^((?:0|[1-9][0-9]*)\.(?:0|[1-9][0-9]*)\.(?:0|[1-9][0-9]*))(-snapshot)?\+([0-9]{8})\.([1-9][0-9]*)$`)

// ParseCore parses three unsigned decimal components without leading zeroes.
// Components larger than uint64 are rejected.
func ParseCore(s string) (Core, error) {
	match := corePattern.FindStringSubmatch(s)
	if match == nil {
		return Core{}, fmt.Errorf("invalid version core %q", s)
	}
	var c Core
	for i, field := range []*uint64{&c.Major, &c.Minor, &c.Patch} {
		value, err := strconv.ParseUint(match[i+1], 10, 64)
		if err != nil {
			return Core{}, fmt.Errorf("version core component overflows uint64: %w", err)
		}
		*field = value
	}
	return c, nil
}

// String returns the core in canonical decimal form.
func (c Core) String() string { return fmt.Sprintf("%d.%d.%d", c.Major, c.Minor, c.Patch) }

// Bump advances the selected component, resets lower components, and rejects
// unsupported bump values or overflow. Inherit and the empty value keep c.
func (c Core) Bump(b Bump) (Core, error) {
	switch b {
	case "", BumpInherit:
		return c, nil
	case BumpMajor:
		if c.Major == math.MaxUint64 {
			return Core{}, fmt.Errorf("major version overflow")
		}
		return Core{Major: c.Major + 1}, nil
	case BumpMinor:
		if c.Minor == math.MaxUint64 {
			return Core{}, fmt.Errorf("minor version overflow")
		}
		return Core{Major: c.Major, Minor: c.Minor + 1}, nil
	case BumpPatch:
		if c.Patch == math.MaxUint64 {
			return Core{}, fmt.Errorf("patch version overflow")
		}
		c.Patch++
		return c, nil
	default:
		return Core{}, fmt.Errorf("invalid version bump %q", b)
	}
}

// Version identifies one application build. Date is a calendar date, PR is a
// positive pull request number, and Snapshot distinguishes untagged source builds.
// Values returned by Parse and Next satisfy these constraints.
type Version struct {
	Core     Core
	Date     time.Time
	PR       uint64
	Snapshot bool
}

// Parse parses an application release or snapshot version without a Git tag
// prefix. It rejects unknown versions, invalid calendar dates, and overflow.
func Parse(s string) (Version, error) {
	match := versionPattern.FindStringSubmatch(s)
	if match == nil {
		return Version{}, fmt.Errorf("invalid application version %q", s)
	}
	core, err := ParseCore(match[1])
	if err != nil {
		return Version{}, err
	}
	date, err := time.Parse("20060102", match[3])
	if err != nil || date.Year() < 1 {
		return Version{}, fmt.Errorf("invalid application version date %q", match[3])
	}
	pr, err := strconv.ParseUint(match[4], 10, 64)
	if err != nil {
		return Version{}, fmt.Errorf("application PR overflows uint64: %w", err)
	}
	return Version{Core: core, Date: date, PR: pr, Snapshot: match[2] != ""}, nil
}

// ParseTag parses a canonical v-prefixed application release tag or a legacy
// build/ tag. The bool reports a legacy tag, including when its suffix is invalid.
// Snapshot tags and ordinary module release tags are rejected.
func ParseTag(s string) (Version, bool, error) {
	legacy := strings.HasPrefix(s, "build/")
	suffix := ""
	switch {
	case legacy:
		suffix = strings.TrimPrefix(s, "build/")
	case strings.HasPrefix(s, "v"):
		suffix = strings.TrimPrefix(s, "v")
	default:
		return Version{}, false, fmt.Errorf("invalid application tag %q", s)
	}
	v, err := Parse(suffix)
	if err != nil {
		return Version{}, legacy, err
	}
	if v.Snapshot {
		return Version{}, legacy, fmt.Errorf("snapshot versions cannot be release tags")
	}
	return v, legacy, nil
}

// String formats the value as an application version. It does not validate a
// manually constructed Version; Tag validates before producing a release tag.
func (v Version) String() string {
	snapshot := ""
	if v.Snapshot {
		snapshot = "-snapshot"
	}
	return fmt.Sprintf("%s%s+%s.%d", v.Core, snapshot, v.Date.Format("20060102"), v.PR)
}

// Tag returns the canonical immutable application release tag. Snapshots and
// invalid manually constructed values cannot become release tags.
func (v Version) Tag() (string, error) {
	if v.Snapshot {
		return "", fmt.Errorf("snapshot versions cannot be release tags")
	}
	if v.Date.IsZero() {
		return "", fmt.Errorf("application version date is missing")
	}
	if _, err := Parse(v.String()); err != nil {
		return "", err
	}
	return "v" + v.String(), nil
}

// Next allocates a release identity using commitTime's calendar date in
// America/Sao_Paulo. The time must be present, PR positive, and bump valid.
// This pure calculation does not inspect Git or create a tag.
func Next(previous Core, commitTime time.Time, pr uint64, bump Bump) (Version, error) {
	if commitTime.IsZero() {
		return Version{}, fmt.Errorf("commit time is missing")
	}
	if pr == 0 {
		return Version{}, fmt.Errorf("PR must be positive")
	}
	core, err := previous.Bump(bump)
	if err != nil {
		return Version{}, err
	}
	location, err := time.LoadLocation("America/Sao_Paulo")
	if err != nil {
		return Version{}, fmt.Errorf("load allocation timezone: %w", err)
	}
	local := commitTime.In(location)
	if local.Year() < 1 || local.Year() > 9999 {
		return Version{}, fmt.Errorf("commit date cannot be represented as YYYYMMDD")
	}
	// Store the selected calendar date independently of timezone transitions.
	// Local midnight may not exist on a daylight-saving transition day.
	date := time.Date(local.Year(), local.Month(), local.Day(), 0, 0, 0, 0, time.UTC)
	return Version{Core: core, Date: date, PR: pr}, nil
}

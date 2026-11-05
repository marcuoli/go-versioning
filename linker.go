package versioning

import (
	"fmt"
	"strconv"
	"strings"
)

// Linker storage belongs to this library so build adapters do not duplicate
// parsing or variable-name conventions in every application. Only explicit
// build-time flags fill it; runtime reads never allocate or access a counter.
var injectedVersion, injectedCommit, injectedBuildTime string
var injectedBuildNumber, injectedCounterID string

// CurrentBuild reads metadata injected by Info.LinkerFlags and otherwise uses
// read-only unknown/VCS identity defaults. Missing actual build time stays
// unknown instead of borrowing a commit timestamp. It never reads a store.
func CurrentBuild() (Info, error) {
	number, err := ParseBuildNumber(injectedBuildNumber)
	if err != nil {
		return Info{}, err
	}
	info, err := Current(Info{Version: injectedVersion, Commit: injectedCommit, BuildTime: injectedBuildTime, OverallBuildNumber: number, BuildCounterID: injectedCounterID})
	// VCS time identifies a commit, not compilation. The new shared adapter only
	// exposes an actual build time when the builder injected one; Current keeps
	// its existing embedded-VCS compatibility contract for established callers.
	if injectedBuildTime == "" {
		info.BuildTime = ""
	}
	return info, err
}

// LinkerFlags returns a go build -ldflags argument for this package's metadata.
// Complete validated injection keeps actual build time separate from commit time
// and cannot be replaced by a dependency module's version at runtime.
func (info Info) LinkerFlags() (string, error) {
	validated, err := validateInfo(info, false)
	if err != nil {
		return "", err
	}
	if !complete(validated) {
		return "", fmt.Errorf("linker metadata requires explicit version, commit and build time")
	}
	const prefix = "github.com/marcuoli/go-versioning."
	values := [][2]string{{"injectedVersion", validated.Version}, {"injectedCommit", validated.Commit}, {"injectedBuildTime", validated.BuildTime}, {"injectedBuildNumber", strconv.FormatUint(validated.OverallBuildNumber, 10)}, {"injectedCounterID", validated.BuildCounterID}}
	flags := make([]string, 0, len(values))
	for _, field := range values {
		flags = append(flags, "-X="+prefix+field[0]+"="+field[1])
	}
	return strings.Join(flags, " "), nil
}

// Package versioning implements immutable application versions and build metadata.
//
// Application versions have the form MAJOR.MINOR.PATCH+YYYYMMDD.PR. Source
// snapshots insert -snapshot before the plus sign. These application identities
// are distinct from Go's canonical installation coordinates, which may be
// pseudo-versions when querying full application tags. Keep displayed identity.
// Next selects the calendar date of the final commit in America/Sao_Paulo.
//
// Applications can inject Info fields through their own linker variables and
// pass them to Current. Runtime discovery reads only the main executable's Go
// build information; it never inspects a working directory or a dependency's
// repository. Repository inspection and tag allocation belong to gitversion.
// CurrentBuild reads library-owned linker fields; Info.LinkerFlags prepares them.
// Explicit persisted build-attempt allocation belongs to buildcounter, never runtime.
package versioning

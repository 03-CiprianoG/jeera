// Package version exposes Jeera's build identity. The values are overridable at
// link time (GoReleaser sets them via -ldflags) and default to a development
// build of the next release.
package version

// These are vars (not consts) so a release build can stamp them with
// -ldflags "-X .../version.Version=... -X .../version.Commit=... -X .../version.Date=...".
var (
	// Version is the SemVer string of this build.
	Version = "0.7.0"
	// Commit is the git SHA the binary was built from.
	Commit = ""
	// Date is the build timestamp (RFC3339).
	Date = ""
)

// String renders a human-readable version line, including commit and date when
// a release build has stamped them.
func String() string {
	s := "jeera " + Version
	if Commit != "" {
		s += " (" + Commit
		if Date != "" {
			s += ", " + Date
		}
		s += ")"
	}
	return s
}

// Short returns just the SemVer string.
func Short() string { return Version }

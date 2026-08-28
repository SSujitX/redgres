// Package buildinfo holds release identity stamped at link time.
package buildinfo

// Version is the product semver (X.Y.Z) from the repository VERSION file.
// Release builds set it via:
//
//	-ldflags "-X github.com/SSujitX/redgres/internal/buildinfo.Version=1.2.3"
//
// Unstamped binaries report "dev".
var Version = "dev"

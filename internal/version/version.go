// Package version exposes the Redgres release version for operator display.
//
// Version defaults to "dev" for untagged local builds and is intended to be
// overridden at build time:
//
//	go build -ldflags "-X github.com/SSujitX/redgres/internal/version.Version=v1.2.3"
package version

// Version is the Redgres release version string. "dev" means an untagged
// local build; release pipelines set it via -ldflags.
var Version = "dev"

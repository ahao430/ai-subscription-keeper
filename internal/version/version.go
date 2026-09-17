// Package version holds the build version. Release builds inject it via
// -ldflags "-X ai-subscription-keeper/internal/version.Version=v1.0.0".
package version

// Version is "dev" for local builds; replaced by the toolchain on release.
var Version = "dev"

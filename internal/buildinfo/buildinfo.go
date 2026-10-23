// Package buildinfo carries the version string stamped into the binary at
// link time. Images built by CI pass
// -ldflags "-X github.com/anxuanzi/QuillDNS/internal/buildinfo.Version=..."
// so the console, /api/v1/health and the quilldns_build_info metric report
// the exact build that is running. Local builds keep the "dev" default.
package buildinfo

// Version is the semantic version or commit the binary was built from.
var Version = "dev"

package buildinfo

// These values are replaced by release builds with linker flags.
var (
	Version   = "development"
	Commit    = "unknown"
	BuildTime = "unknown"
)

// Info describes the binary serving an API response.
type Info struct {
	Version   string `json:"version"`
	Commit    string `json:"commit"`
	BuildTime string `json:"buildTime"`
}

// Current returns the build metadata for this binary.
func Current() Info {
	return Info{Version: Version, Commit: Commit, BuildTime: BuildTime}
}

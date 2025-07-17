package version

import "fmt"

var (
	// Version is the version of the application
	Version = ""
	// GitCommit is the git commit hash of the application
	GitCommit = ""
)

// Info is the version information of the application
type Info struct {
	Version   string `json:"version"`
	GitCommit string `json:"git_commit"`
}

// NewInfo returns a new Info instance
func NewInfo() *Info {
	return &Info{
		Version:   Version,
		GitCommit: GitCommit,
	}
}

// String returns the string representation of the Info
func (v *Info) String() string {
	return fmt.Sprintf("Version: %s\nGit Commit: %s", v.Version, v.GitCommit)
}

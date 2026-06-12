package version

import "runtime"

var (
	Version   = "dev"
	Commit    = "unknown"
	BuildDate = "unknown"
)

type Info struct {
	AppName   string `json:"app_name"`
	Version   string `json:"version"`
	Commit    string `json:"commit"`
	BuildDate string `json:"build_date"`
	GoVersion string `json:"go_version"`
}

func Current() Info {
	return Info{
		AppName:   "KoalaBye",
		Version:   Version,
		Commit:    Commit,
		BuildDate: BuildDate,
		GoVersion: runtime.Version(),
	}
}

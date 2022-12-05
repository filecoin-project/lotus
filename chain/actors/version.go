package actors

type Version int

/* inline-gen template

var LatestVersion = {{.latestActorsVersion}}

var Versions = []int{ {{range .actorVersions}} {{.}}, {{end}} }

const ({{range .actorVersions}}
	Version{{.}} Version = {{.}}{{end}}
)

/* inline-gen start */

var LatestVersion = 10

var Versions = []int{0, 2, 3, 4, 5, 6, 7, 8, 9, 10}

const (
	Version0  Version = 0
	Version2  Version = 2
	Version3  Version = 3
	Version4  Version = 4
	Version5  Version = 5
	Version6  Version = 6
	Version7  Version = 7
	Version8  Version = 8
	Version9  Version = 9
	Version10 Version = 10
)

/* inline-gen end */

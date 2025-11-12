package actors

type Version int

/* inline-gen template

var LatestVersion = {{.latestActorsVersion}}

var Versions = []int{ {{range .actorVersions}} {{.}}, {{end}} }

const ({{range .actorVersions}}
	Version{{.}} Version = {{.}}{{end}}
)

/* inline-gen start */

var LatestVersion = 18

var Versions = []int{0, 2, 3, 4, 5, 6, 7, 8, 9, 10, 11, 12, 13, 14, 15, 16, 17, 18}

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
	Version11 Version = 11
	Version12 Version = 12
	Version13 Version = 13
	Version14 Version = 14
	Version15 Version = 15
	Version16 Version = 16
	Version17 Version = 17
	Version18 Version = 18
)

/* inline-gen end */

package buildconstants

type DrandEnum int

const (
	DrandMainnet DrandEnum = iota + 1
	DrandTestnet
	DrandDevnet
	DrandLocalnet
	DrandIncentinet
	DrandQuicknet
)

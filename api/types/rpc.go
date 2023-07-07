package apitypes

type Aliaser interface {
	AliasMethod(alias, original string)
}

package storiface

type PathType string

const (
	PathStorage PathType = "storage"
	PathSealing PathType = "sealing"

	PathStoragePrefer PathType = "storage-prefer"
)

type AcquireMode string

const (
	AcquireMove AcquireMode = "move"
	AcquireCopy AcquireMode = "copy"
)

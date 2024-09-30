package sealing

//go:generate go run github.com/golang/mock/mockgen -destination=mocks/mocks.go -package=mocks . CommitBatcherApi,PreCommitBatcherApi,SealingAPI,Context

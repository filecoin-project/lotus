package kit

type TestDealState int

const (
	TestDealStateFailed     = TestDealState(-1)
	TestDealStateInProgress = TestDealState(0)
	TestDealStateComplete   = TestDealState(1)
)

// CategorizeDealState categorizes deal states into one of three states:
// Complete, InProgress, Failed.
func CategorizeDealState(dealStatus string) TestDealState {
	switch dealStatus {
	case "StorageDealFailing", "StorageDealError":
		return TestDealStateFailed
	case "StorageDealAwaitingPreCommit", "StorageDealSealing", "StorageDealActive", "StorageDealExpired", "StorageDealSlashed":
		return TestDealStateComplete
	}
	return TestDealStateInProgress
}

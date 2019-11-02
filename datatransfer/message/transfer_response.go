package message

// transferResponse is a private struct that satisfies the DataTransferResponse interface
type transferResponse struct {
	Acpt bool
}

// 	Accepted returns true if the request is accepted in the response
func (trsp *transferResponse) Accepted() bool {
	return trsp.Acpt
}

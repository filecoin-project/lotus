package message

import "io"

// transferResponse is a private struct that satisfies the DataTransferResponse interface
type transferResponse struct {
	Acpt bool
}

// 	Accepted returns true if the request is accepted in the response
func (trsp *transferResponse) Accepted() bool {
	return trsp.Acpt
}

// ToNet serializes a transfer response. It's a wrapper for MarshalCBOR to provide
// symmetry with FromNet
func (trsp *transferResponse) ToNet(w io.Writer) error {
	return trsp.MarshalCBOR(w)
}

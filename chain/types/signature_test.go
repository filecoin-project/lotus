package types

import (
	"bytes"
	"testing"
)

func TestSignatureSerializeRoundTrip(t *testing.T) {
	s := &Signature{
		Data: []byte("foo bar cat dog"),
		Type: KTBLS,
	}

	buf := new(bytes.Buffer)
	if err := s.MarshalCBOR(buf); err != nil {
		t.Fatal(err)
	}

	var outs Signature
	if err := outs.UnmarshalCBOR(buf); err != nil {
		t.Fatal(err)
	}

	if !outs.Equals(s) {
		t.Fatal("serialization round trip failed")
	}
}

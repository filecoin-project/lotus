package evmstorage

import (
	"encoding/json"
	"fmt"
	"math/big"
	"os"
	"strings"
)

// Layout is a solc/forge storage layout ("forge inspect <Contract>
// storageLayout --json"): the contract's top-level variables and a table
// describing every type they reference. This, not the ABI, is what maps
// storage slots to names; ABIs describe calldata and events only.
type Layout struct {
	Storage []*LayoutEntry         `json:"storage"`
	Types   map[string]*LayoutType `json:"types"`
}

// LayoutEntry places one variable (or struct member) at a slot and byte
// offset. Member slots are relative to the enclosing struct's base.
type LayoutEntry struct {
	Label  string      `json:"label"`
	Offset int         `json:"offset"`
	Slot   json.Number `json:"slot"`
	Type   string      `json:"type"`
}

// LayoutType describes one type from the layout's type table.
type LayoutType struct {
	// Encoding is one of inplace, mapping, dynamic_array or bytes.
	Encoding      string         `json:"encoding"`
	Label         string         `json:"label"`
	NumberOfBytes json.Number    `json:"numberOfBytes"`
	Key           string         `json:"key,omitempty"`   // mapping
	Value         string         `json:"value,omitempty"` // mapping
	Base          string         `json:"base,omitempty"`  // array element
	Members       []*LayoutEntry `json:"members,omitempty"`
}

// LoadLayout reads a forge storage layout JSON file.
func LoadLayout(path string) (*Layout, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return nil, err
	}
	var l Layout
	if err := json.Unmarshal(data, &l); err != nil {
		return nil, fmt.Errorf("parsing storage layout %s: %w", path, err)
	}
	if len(l.Storage) == 0 || len(l.Types) == 0 {
		return nil, fmt.Errorf("%s does not look like a forge storage layout (want top-level \"storage\" and \"types\")", path)
	}
	for _, e := range l.Storage {
		if _, ok := l.Types[e.Type]; !ok {
			return nil, fmt.Errorf("layout references type %s with no entry in the types table", e.Type)
		}
	}
	// An elementary value is read as a byte range within a 32-byte slot, so
	// its width must fit. Reject bad sizes here rather than let a slice
	// panic on a malformed layout file.
	for name, t := range l.Types {
		if t.isElementary() {
			if s := t.size(); s < 1 || s > 32 {
				return nil, fmt.Errorf("layout type %s has out-of-range size %d bytes (want 1..32)", name, s)
			}
		}
	}
	return &l, nil
}

func (e *LayoutEntry) slotBig() *big.Int {
	n := new(big.Int)
	n.SetString(e.Slot.String(), 10)
	return n
}

func (t *LayoutType) size() int {
	n, _ := t.NumberOfBytes.Int64()
	return int(n)
}

// isElementary reports whether the type decodes as a single scalar
// (address, uintN, bool, enum, fixed bytes) rather than a container.
func (t *LayoutType) isElementary() bool {
	return t.Encoding == "inplace" && len(t.Members) == 0 && t.Base == ""
}

// decodeElementary renders the size() bytes of an in-place value
// extracted from a slot. Addresses and fixed bytes as 0x hex, booleans as
// booleans, integers as decimal strings (they can exceed float64 and
// json.Number keeps them exact).
func (t *LayoutType) decodeElementary(b []byte) interface{} {
	switch {
	case t.Label == "bool":
		return len(b) > 0 && b[len(b)-1] != 0
	case strings.HasPrefix(t.Label, "address") || strings.HasPrefix(t.Label, "contract "):
		return fmt.Sprintf("0x%040x", new(big.Int).SetBytes(b))
	case strings.HasPrefix(t.Label, "bytes"):
		return fmt.Sprintf("0x%x", b)
	case strings.HasPrefix(t.Label, "int"):
		n := new(big.Int).SetBytes(b)
		// two's complement: high bit of the type's width means negative
		if len(b) > 0 && b[0]&0x80 != 0 {
			n.Sub(n, new(big.Int).Lsh(big.NewInt(1), uint(len(b))*8))
		}
		return json.Number(n.String())
	default: // uintN, enum
		return json.Number(new(big.Int).SetBytes(b).String())
	}
}

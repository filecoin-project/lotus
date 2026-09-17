package evmstorage

import (
	"bytes"
	"encoding/hex"
	"encoding/json"
	"math/big"
	"os"
	"testing"

	"github.com/stretchr/testify/require"
)

// testLayout builds a layout exercising every encoding: packed scalars,
// strings (short and long), a dynamic array, a struct-valued mapping, a
// nested mapping and a string-keyed mapping fed by a sibling keys array.
func testLayout() *Layout {
	return &Layout{
		Storage: []*LayoutEntry{
			{Label: "count", Slot: "0", Offset: 0, Type: "t_uint64"},
			{Label: "flag", Slot: "0", Offset: 8, Type: "t_bool"},
			{Label: "owner", Slot: "1", Offset: 0, Type: "t_address"},
			{Label: "name", Slot: "2", Offset: 0, Type: "t_string"},
			{Label: "motto", Slot: "3", Offset: 0, Type: "t_string"},
			{Label: "ids", Slot: "4", Offset: 0, Type: "t_array_u32"},
			{Label: "things", Slot: "5", Offset: 0, Type: "t_map_thing"},
			{Label: "grid", Slot: "6", Offset: 0, Type: "t_map_map"},
			{Label: "meta", Slot: "7", Offset: 0, Type: "t_map_meta"},
			{Label: "metaKeys", Slot: "8", Offset: 0, Type: "t_map_keys"},
			{Label: "balances", Slot: "9", Offset: 0, Type: "t_map_addr"},
		},
		Types: map[string]*LayoutType{
			"t_uint64":  {Encoding: "inplace", Label: "uint64", NumberOfBytes: "8"},
			"t_bool":    {Encoding: "inplace", Label: "bool", NumberOfBytes: "1"},
			"t_address": {Encoding: "inplace", Label: "address", NumberOfBytes: "20"},
			"t_uint256": {Encoding: "inplace", Label: "uint256", NumberOfBytes: "32"},
			"t_uint32":  {Encoding: "inplace", Label: "uint32", NumberOfBytes: "4"},
			"t_string":  {Encoding: "bytes", Label: "string", NumberOfBytes: "32"},

			"t_array_u32": {Encoding: "dynamic_array", Label: "uint32[]", NumberOfBytes: "32", Base: "t_uint32"},
			"t_array_str": {Encoding: "dynamic_array", Label: "string[]", NumberOfBytes: "32", Base: "t_string"},

			"t_thing": {Encoding: "inplace", Label: "struct Thing", NumberOfBytes: "64", Members: []*LayoutEntry{
				{Label: "who", Slot: "0", Offset: 0, Type: "t_address"},
				{Label: "size", Slot: "1", Offset: 0, Type: "t_uint256"},
			}},
			"t_map_thing": {Encoding: "mapping", Label: "mapping(uint256 => struct Thing)", NumberOfBytes: "32", Key: "t_uint256", Value: "t_thing"},
			"t_map_inner": {Encoding: "mapping", Label: "mapping(uint256 => uint256)", NumberOfBytes: "32", Key: "t_uint256", Value: "t_uint256"},
			"t_map_map":   {Encoding: "mapping", Label: "mapping(uint256 => mapping(uint256 => uint256))", NumberOfBytes: "32", Key: "t_uint256", Value: "t_map_inner"},
			"t_map_str":   {Encoding: "mapping", Label: "mapping(string => string)", NumberOfBytes: "32", Key: "t_string", Value: "t_string"},
			"t_map_meta":  {Encoding: "mapping", Label: "mapping(uint256 => mapping(string => string))", NumberOfBytes: "32", Key: "t_uint256", Value: "t_map_str"},
			"t_map_keys":  {Encoding: "mapping", Label: "mapping(uint256 => string[])", NumberOfBytes: "32", Key: "t_uint256", Value: "t_array_str"},
			"t_map_addr":  {Encoding: "mapping", Label: "mapping(address => uint256)", NumberOfBytes: "32", Key: "t_address", Value: "t_uint256"},
		},
	}
}

func slotN(n int64) [32]byte { return slotOf(big.NewInt(n)) }

func val(b ...byte) [32]byte {
	var out [32]byte
	copy(out[32-len(b):], b)
	return out
}

func mapSlot(base [32]byte, intKey uint64) [32]byte {
	var kb [32]byte
	new(big.Int).SetUint64(intKey).FillBytes(kb[:])
	return keccakSlot(kb[:], base[:])
}

func strMapSlot(base [32]byte, key string) [32]byte {
	return keccakSlot([]byte(key), base[:])
}

func TestDecodeStaticAndMappings(t *testing.T) {
	slots := make(map[[32]byte][32]byte)

	// count=7 packed with flag=true
	v := val()
	v[31], v[30], v[29], v[28], v[27], v[26], v[25], v[24] = 7, 0, 0, 0, 0, 0, 0, 0
	v[23] = 1 // flag at offset 8
	slots[slotN(0)] = v

	var addr [32]byte
	for i := 12; i < 32; i++ {
		addr[i] = byte(i)
	}
	slots[slotN(1)] = addr

	// short string "hi" (len 2 -> final byte 4)
	name := val()
	copy(name[:], "hi")
	name[31] = 4
	slots[slotN(2)] = name

	// long string: 40 bytes of 'x', length slot = 2*40+1
	motto := val()
	new(big.Int).SetInt64(81).FillBytes(motto[:])
	slots[slotN(3)] = motto

	s3 := slotN(3)
	mottoData := keccakSlot(s3[:])
	var chunk1, chunk2 [32]byte
	for i := range chunk1 {
		chunk1[i] = 'x'
	}
	for i := 0; i < 8; i++ {
		chunk2[i] = 'x'
	}
	slots[mottoData] = chunk1
	slots[addSlot(mottoData, big.NewInt(1))] = chunk2

	// ids = [1,2,3] packed uint32, 8 per slot
	lenSlot := val()
	lenSlot[31] = 3
	slots[slotN(4)] = lenSlot
	s4 := slotN(4)
	idsData := keccakSlot(s4[:])
	ids := val()
	ids[31], ids[27], ids[23] = 1, 2, 3
	slots[idsData] = ids

	// things[42] = Thing{who: addr, size: 99}
	tbase := mapSlot(slotN(5), 42)
	slots[tbase] = addr
	slots[addSlot(tbase, big.NewInt(1))] = val(99)

	// grid[42][7] = 5 (42 becomes strong via things hit)
	g1 := mapSlot(slotN(6), 42)
	slots[mapSlot(g1, 7)] = val(5)

	// metaKeys[42] = ["k1"]; meta[42]["k1"] = "v1"
	mkBase := mapSlot(slotN(8), 42)
	mkLen := val()
	mkLen[31] = 1
	slots[mkBase] = mkLen
	k1 := val()
	copy(k1[:], "k1")
	k1[31] = 4
	slots[keccakSlot(mkBase[:])] = k1
	metaOuter := mapSlot(slotN(7), 42)
	v1 := val()
	copy(v1[:], "v1")
	v1[31] = 4
	slots[strMapSlot(metaOuter, "k1")] = v1

	// balances[addr] = 1000; addr appears as a value in slot 1
	s9 := slotN(9)
	slots[keccakSlot(addr[:], s9[:])] = val(0x03, 0xe8)

	d := NewDecoder(testLayout(), slots)
	ann := d.Decode(Hints{}, 128, DefaultNestedProbeRange)

	get := func(s [32]byte) *Annotation {
		a := ann[s]
		require.NotNil(t, a, "expected annotation for slot %x", s)
		return a
	}

	packed := get(slotN(0))
	require.Equal(t, "packed", packed.Type)
	dec := packed.Decoded.(map[string]interface{})
	require.Equal(t, json.Number("7"), dec["count"])
	require.Equal(t, true, dec["flag"])

	require.Equal(t, "owner", get(slotN(1)).Label)
	require.Equal(t, "hi", get(slotN(2)).Decoded)
	require.Equal(t, "motto.length", get(slotN(3)).Label)
	require.Contains(t, get(mottoData).Label, "motto.data")
	require.Equal(t, "ids.length", get(slotN(4)).Label)
	require.Equal(t, "ids[0..2]", get(idsData).Label)

	require.Equal(t, "things[42].who", get(tbase).Label)
	require.Equal(t, "things[42].size", get(addSlot(tbase, big.NewInt(1))).Label)
	require.Equal(t, "grid[42][7]", get(mapSlot(g1, 7)).Label)
	require.Equal(t, "metaKeys[42].length", get(mkBase).Label)
	require.Equal(t, `meta[42]["k1"]`, get(strMapSlot(metaOuter, "k1")).Label)

	cov := d.Coverage()
	require.Equal(t, len(slots), cov.Slots)
	require.Equal(t, len(slots), cov.Annotated, "every slot should be labelled")
}

// TestMalformedLayoutSizes exercises the defensive bounds: a
// programmatically supplied bad elementary size must not panic the
// decoder (the CLI rejects these at LoadLayout; NewDecoder callers get
// graceful skips).
func TestMalformedLayoutSizes(t *testing.T) {
	slots := map[[32]byte][32]byte{slotN(0): val(1), slotN(1): val(2)}
	layout := &Layout{
		Storage: []*LayoutEntry{
			{Label: "zero", Slot: "0", Offset: 0, Type: "t_zero"},
			{Label: "over", Slot: "1", Offset: 0, Type: "t_over"},
			{Label: "arr", Slot: "2", Offset: 0, Type: "t_arr_zero"},
		},
		Types: map[string]*LayoutType{
			"t_zero":     {Encoding: "inplace", Label: "uint0", NumberOfBytes: "0"},
			"t_over":     {Encoding: "inplace", Label: "big", NumberOfBytes: "40"},
			"t_arr_zero": {Encoding: "dynamic_array", Label: "uint0[]", NumberOfBytes: "32", Base: "t_zero"},
		},
	}
	require.NotPanics(t, func() {
		NewDecoder(layout, slots).Decode(Hints{}, 4, DefaultNestedProbeRange)
	})
}

func TestWellKnownSlots(t *testing.T) {
	slots := make(map[[32]byte][32]byte)
	impl, _ := new(big.Int).SetString("360894a13ba1a3210667c828492db98dca3e2076cc3735a920a3ca505d382bbc", 16)
	slots[slotOf(impl)] = val(0xde, 0xad)

	// erc7201(openzeppelin.storage.Ownable)
	h := keccakSlot([]byte("openzeppelin.storage.Ownable"))
	n := new(big.Int).SetBytes(h[:])
	n.Sub(n, big.NewInt(1))
	var pre [32]byte
	n.FillBytes(pre[:])
	base := keccakSlot(pre[:])
	base[31] = 0
	slots[base] = val(0xbe, 0xef)

	d := NewDecoder(&Layout{
		Storage: []*LayoutEntry{{Label: "x", Slot: "0", Offset: 0, Type: "t_uint256"}},
		Types:   map[string]*LayoutType{"t_uint256": {Encoding: "inplace", Label: "uint256", NumberOfBytes: "32"}},
	}, slots)
	ann := d.Decode(Hints{}, 0, DefaultNestedProbeRange)

	require.Equal(t, "erc1967.implementation", ann[slotOf(impl)].Label)
	require.Equal(t, "openzeppelin.storage.Ownable._owner", ann[base].Label)
}

// TestProviderIdSetGolden decodes a real calibnet dump (FOC endorsed
// providers list, 3 slots, captured 2026-07-07) against its forge layout.
func TestProviderIdSetGolden(t *testing.T) {
	layout, err := LoadLayout("testdata/provideridset.layout.json")
	require.NoError(t, err)

	slots := make(map[[32]byte][32]byte)
	raw, err := os.ReadFile("testdata/provideridset-calib.ndjson")
	require.NoError(t, err)
	type record struct{ Slot, Value string }
	labels := make(map[string]string)
	var order [][32]byte
	dec := json.NewDecoder(bytes.NewReader(raw))
	for dec.More() {
		var r record
		require.NoError(t, dec.Decode(&r))
		var s, v [32]byte
		sb, _ := hex.DecodeString(r.Slot[2:])
		vb, _ := hex.DecodeString(r.Value[2:])
		copy(s[:], sb)
		copy(v[:], vb)
		slots[s] = v
		order = append(order, s)
	}
	require.Len(t, slots, 3)

	d := NewDecoder(layout, slots)
	ann := d.Decode(Hints{}, 8, DefaultNestedProbeRange)
	for _, s := range order {
		require.NotNil(t, ann[s])
		labels[ann[s].Label] = ""
	}
	require.Contains(t, labels, "_owner")
	require.Contains(t, labels, "list.length")
	require.Contains(t, labels, "list[0]")
}

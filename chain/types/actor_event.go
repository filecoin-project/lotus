package types

import (
	"encoding/base64"
	"errors"
	"fmt"

	"github.com/ipfs/go-cid"
	"github.com/ipld/go-ipld-prime"
	"github.com/ipld/go-ipld-prime/codec"
	"github.com/ipld/go-ipld-prime/codec/dagcbor"
	"github.com/ipld/go-ipld-prime/codec/dagjson"
	"github.com/ipld/go-ipld-prime/codec/raw"
	"github.com/ipld/go-ipld-prime/datamodel"
	"github.com/ipld/go-ipld-prime/fluent/qp"
	basicnode "github.com/ipld/go-ipld-prime/node/basic"
	"github.com/ipld/go-ipld-prime/node/bindnode"
	"github.com/ipld/go-ipld-prime/schema"

	"github.com/filecoin-project/go-address"
	"github.com/filecoin-project/go-state-types/abi"
)

type ActorEventBlock struct {
	// The value codec to match when filtering event values.
	Codec uint64 `json:"codec"`

	// The value to want to match on associated with the corresponding "event key"
	// when filtering events.
	// Should be a byte array encoded with the specified codec.
	// Assumes base64 encoding when converting to/from JSON strings.
	Value []byte `json:"value"`
}

type ActorEventFilter struct {
	// Matches events from one of these actors, or any actor if empty.
	// For now, this MUST be a Filecoin address.
	Addresses []address.Address `json:"addresses,omitempty"`

	// Matches events with the specified key/values, or all events if empty.
	// If the value is an empty slice, the filter will match on the key only, accepting any value.
	Fields map[string][]ActorEventBlock `json:"fields,omitempty"`

	// The height of the earliest tipset to include in the query. If empty, the query starts at the
	// last finalized tipset.
	// NOTE: In a future upgrade, this will be strict when set and will result in an error if a filter
	// cannot be fulfilled by the depth of history available in the node. Currently, the node will
	// nott return an error, but will return starting from the epoch it has data for.
	FromHeight *abi.ChainEpoch `json:"fromHeight,omitempty"`

	// The height of the latest tipset to include in the query. If empty, the query ends at the
	// latest tipset.
	ToHeight *abi.ChainEpoch `json:"toHeight,omitempty"`

	// Restricts events returned to those emitted from messages contained in this tipset.
	// If `TipSetKey` is legt empty in the filter criteria, then neither `FromHeight` nor `ToHeight` are allowed.
	TipSetKey *TipSetKey `json:"tipsetKey,omitempty"`
}

type ActorEvent struct {
	encodeCompact *bool `json:"-"` // shouldn't be exposed publicly for any reason

	// Event entries in log form.
	Entries []EventEntry `json:"entries"`

	// Filecoin address of the actor that emitted this event.
	// NOTE: In a future upgrade, this will change to always be an ID address. Currently this will be
	// either the f4 address, or ID address if an f4 is not available for this actor.
	Emitter address.Address `json:"emitter"`

	// Reverted is set to true if the message that produced this event was reverted because of a network re-org
	// in that case, the event should be considered as reverted as well.
	Reverted bool `json:"reverted"`

	// Height of the tipset that contained the message that produced this event.
	Height abi.ChainEpoch `json:"height"`

	// The tipset that contained the message that produced this event.
	TipSetKey TipSetKey `json:"tipsetKey"`

	// CID of message that produced this event.
	MsgCid cid.Cid `json:"msgCid"`
}

// AsCompactEncoded will trigger alternate JSON encoding for ActorEvents, where the event entries
// are encoded as a list of tuple representation structs, rather than a list of maps, values are
// decoded using the specified codec where possible, and they are encoded using dag-json form so
// bytes are represented using the `{"/":{"bytes":"base64"}}` form rather than Go standard base64
// encoding.
func (ae ActorEvent) AsCompactEncoded() ActorEvent {
	ae.encodeCompact = new(bool)
	*ae.encodeCompact = true
	return ae
}

func (ae *ActorEvent) UnmarshalJSON(b []byte) error {
	nd, err := ipld.Decode(b, dagjson.Decode)
	if err != nil {
		return err
	}
	builder := actorEventProto.Representation().NewBuilder()
	if err := builder.AssignNode(nd); err != nil {
		return err
	}
	aePtr := bindnode.Unwrap(builder.Build())
	aec, _ := aePtr.(*ActorEvent) // safe to assume type
	*ae = *aec

	// check if we were encoded in compact form and set the flag accordingly
	entries, _ := nd.LookupByString("entries")
	if entries.Length() > 0 {
		first, _ := entries.LookupByIndex(0)
		if first.Kind() == datamodel.Kind_List {
			ae.encodeCompact = new(bool)
			*ae.encodeCompact = true
		}
	}

	return nil
}

func (ae ActorEvent) MarshalJSON() ([]byte, error) {
	var entryOpt bindnode.Option = eventEntryBindnodeOption
	if ae.encodeCompact != nil {
		if *ae.encodeCompact {
			entryOpt = eventEntryCompactBindnodeOption
		}
		ae.encodeCompact = nil // hide it from this encode
	}
	nd := bindnode.Wrap(
		&ae,
		actorEventProto.Type(),
		TipSetKeyAsLinksListBindnodeOption,
		addressAsStringBindnodeOption,
		entryOpt,
	)
	return ipld.Encode(nd, dagjson.Encode)
}

// TODO: move this in to go-state-types/ipld with the address "as bytes" form
var addressAsStringBindnodeOption = bindnode.TypedStringConverter(&address.Address{}, addressFromString, addressToString)

func addressFromString(s string) (interface{}, error) {
	a, err := address.NewFromString(s)
	if err != nil {
		return nil, err
	}
	return &a, nil
}

func addressToString(iface interface{}) (string, error) {
	addr, ok := iface.(*address.Address)
	if !ok {
		return "", errors.New("expected *Address value")
	}
	return addr.String(), nil
}

var eventEntryBindnodeOption = bindnode.TypedAnyConverter(&EventEntry{}, eventEntryFromAny, eventEntryToAny)
var eventEntryCompactBindnodeOption = bindnode.TypedAnyConverter(&EventEntry{}, eventEntryCompactFromAny, eventEntryCompactToAny)

// eventEntryFromAny will instantiate an EventEntry assuming standard Go JSON form, i.e.:
// {"Codec":82,"Flags":0,"Key":"key2","Value":"dmFsdWUy"}
// Where the value is intact as raw bytes but represented as a base64 string, and the object is
// represented as a map.
func eventEntryFromAny(n datamodel.Node) (interface{}, error) {
	if n.Kind() == datamodel.Kind_List {
		return eventEntryCompactFromAny(n)
	}
	if n.Kind() != datamodel.Kind_Map {
		return nil, errors.New("expected map representation for EventEntry")
	}
	if n.Length() != 4 {
		return nil, errors.New("expected 4 fields for EventEntry")
	}
	fn, err := n.LookupByString("Flags")
	if err != nil {
		return nil, fmt.Errorf("missing Flags field for EventEntry: %w", err)
	}
	flags, err := fn.AsInt()
	if err != nil {
		return nil, fmt.Errorf("expected int in Flags field for EventEntry: %w", err)
	}
	cn, err := n.LookupByString("Codec")
	if err != nil {
		return nil, fmt.Errorf("missing Codec field for EventEntry: %w", err)
	}
	codec, err := cn.AsInt()
	if err != nil {
		return nil, fmt.Errorf("expected int in Codec field for EventEntry: %w", err)
	}
	// it has to fit into a uint8
	if flags < 0 || flags > 255 {
		return nil, fmt.Errorf("expected uint8 in Flags field for EventEntry, got %d", flags)
	}
	kn, err := n.LookupByString("Key")
	if err != nil {
		return nil, fmt.Errorf("missing Key field for EventEntry: %w", err)
	}
	key, err := kn.AsString()
	if err != nil {
		return nil, fmt.Errorf("expected string in Key field for EventEntry: %w", err)
	}
	vn, err := n.LookupByString("Value")
	if err != nil {
		return nil, fmt.Errorf("missing Value field for EventEntry: %w", err)
	}
	value64, err := vn.AsString() // base64
	if err != nil {
		return nil, fmt.Errorf("expected string in Value field for EventEntry: %w", err)
	}
	value, err := base64.StdEncoding.DecodeString(value64)
	if err != nil {
		return nil, fmt.Errorf("failed to decode base64 value: %w", err)
	}
	return &EventEntry{
		Flags: uint8(flags),
		Key:   key,
		Codec: uint64(codec),
		Value: value,
	}, nil
}

// eventEntryCompactFromAny will instantiate an EventEntry assuming compact form, i.e.:
// [0,82,"key2",{"/":{"bytes":"dmFsdWUy"}}]
// Where the value is represented in its decoded IPLD data model form, and the object is represented
// as a tuple.
func eventEntryCompactFromAny(n datamodel.Node) (interface{}, error) {
	if n.Kind() != datamodel.Kind_List {
		return nil, errors.New("expected list representation for compact EventEntry")
	}
	if n.Length() != 4 {
		return nil, errors.New("expected 4 fields for EventEntry")
	}
	// Flags before Codec in this form, sorted Codec before Flags in the non-compact form when dag-json
	fn, err := n.LookupByIndex(0)
	if err != nil {
		return nil, fmt.Errorf("missing Flags field for EventEntry: %w", err)
	}
	flags, err := fn.AsInt()
	if err != nil {
		return nil, fmt.Errorf("expected int in Flags field for EventEntry: %w", err)
	}
	// it has to fit into a uint8
	if flags < 0 || flags > 255 {
		return nil, fmt.Errorf("expected uint8 in Flags field for EventEntry, got %d", flags)
	}
	cn, err := n.LookupByIndex(1)
	if err != nil {
		return nil, fmt.Errorf("missing Codec field for EventEntry: %w", err)
	}
	codecCode, err := cn.AsInt()
	if err != nil {
		return nil, fmt.Errorf("expected int in Codec field for EventEntry: %w", err)
	}
	kn, err := n.LookupByIndex(2)
	if err != nil {
		return nil, fmt.Errorf("missing Key field for EventEntry: %w", err)
	}
	key, err := kn.AsString()
	if err != nil {
		return nil, fmt.Errorf("expected string in Key field for EventEntry: %w", err)
	}
	vn, err := n.LookupByIndex(3)
	if err != nil {
		return nil, fmt.Errorf("missing Value field for EventEntry: %w", err)
	}
	// as of writing only 0x55 and 0x51 are supported here, but we'll treat raw as the default,
	// regardless, which means that for any unexpected codecs encountered we'll assume that the
	// encoder also didn't know what to do with it and just treat it as raw bytes.
	var value []byte
	switch codecCode {
	case 0x51: // plain cbor
		if value, err = ipld.Encode(vn, dagcbor.Encode); err != nil {
			return nil, fmt.Errorf("failed to encode cbor value: %w", err)
		}
	default: // raw (0x55) and all unknowns
		if vn.Kind() != datamodel.Kind_Bytes {
			return nil, fmt.Errorf("expected bytes in Value field for EventEntry, got %s", vn.Kind())
		}
		if value, err = vn.AsBytes(); err != nil {
			return nil, err
		}
	}

	return &EventEntry{
		Flags: uint8(flags),
		Key:   key,
		Codec: uint64(codecCode),
		Value: value,
	}, nil
}

// eventEntryToAny does the reverse of eventEntryFromAny, converting an EventEntry back to the
// standard Go JSON form, i.e.:
// {"Codec":82,"Flags":0,"Key":"key2","Value":"dmFsdWUy"}
func eventEntryToAny(iface interface{}) (datamodel.Node, error) {
	ee, ok := iface.(*EventEntry)
	if !ok {
		return nil, errors.New("expected *Address value")
	}
	return qp.BuildMap(basicnode.Prototype.Map, 4, func(ma datamodel.MapAssembler) {
		qp.MapEntry(ma, "Flags", qp.Int(int64(ee.Flags)))
		qp.MapEntry(ma, "Codec", qp.Int(int64(ee.Codec)))
		qp.MapEntry(ma, "Key", qp.String(ee.Key))
		qp.MapEntry(ma, "Value", qp.String(base64.StdEncoding.EncodeToString(ee.Value)))
	})
}

// eventEntryCompactToAny does the reverse of eventEntryCompactFromAny, converting an EventEntry
// back to the compact form, i.e.:
// [0,82,"key2",{"/":{"bytes":"dmFsdWUy"}}]
func eventEntryCompactToAny(iface interface{}) (datamodel.Node, error) {
	ee, ok := iface.(*EventEntry)
	if !ok {
		return nil, errors.New("expected *Address value")
	}
	var decoder codec.Decoder = raw.Decode
	if ee.Codec == 0x51 {
		decoder = dagcbor.Decode
	}
	valueNode, err := ipld.Decode(ee.Value, decoder)
	if err != nil {
		log.Warn("failed to decode event entry value with expected codec", "err", err)
		valueNode = basicnode.NewBytes(ee.Value)
	}
	return qp.BuildList(basicnode.Prototype.List, 4, func(la datamodel.ListAssembler) {
		qp.ListEntry(la, qp.Int(int64(ee.Flags)))
		qp.ListEntry(la, qp.Int(int64(ee.Codec)))
		qp.ListEntry(la, qp.String(ee.Key))
		qp.ListEntry(la, qp.Node(valueNode))
	})
}

var (
	actorEventProto    schema.TypedPrototype
	fullFormIpldSchema = `
type ActorEvent struct {
	encodeCompact optional Bool
	Entries       [Any]        (rename "entries")   # EventEntry
	Emitter       String       (rename "emitter")   # addr.Address
	Reverted      Bool         (rename "reverted")
	Height        Int          (rename "height")
	TipSetKey     Any          (rename "tipsetKey") # types.TipSetKey
	MsgCid        &Any         (rename "msgCid")
}
`
)

func init() {
	typeSystem, err := ipld.LoadSchemaBytes([]byte(fullFormIpldSchema))
	if err != nil {
		panic(err)
	}
	schemaType := typeSystem.TypeByName("ActorEvent")
	if schemaType == nil {
		panic(fmt.Errorf("schema for [%T] does not contain that named type [%s]", (*ActorEvent)(nil), "ActorEvent"))
	}
	actorEventProto = bindnode.Prototype(
		(*ActorEvent)(nil),
		schemaType,
		TipSetKeyAsLinksListBindnodeOption,
		addressAsStringBindnodeOption,
		eventEntryBindnodeOption,
	)
}

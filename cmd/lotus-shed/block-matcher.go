package main

import (
	"bytes"
	"context"
	"fmt"

	blocks "github.com/ipfs/go-block-format"
	cbor "github.com/ipfs/go-ipld-cbor"
	"github.com/ipld/go-ipld-prime"
	"github.com/ipld/go-ipld-prime/codec/dagcbor"
	"github.com/ipld/go-ipld-prime/datamodel"
	"github.com/ipld/go-ipld-prime/node/bindnode"
	"github.com/ipld/go-ipld-prime/schema"
	schemadmt "github.com/ipld/go-ipld-prime/schema/dmt"
	schemadsl "github.com/ipld/go-ipld-prime/schema/dsl"
	"github.com/ipld/go-ipld-prime/traversal"
	cbg "github.com/whyrusleeping/cbor-gen"
	"golang.org/x/xerrors"

	"github.com/filecoin-project/go-amt-ipld/v4"
	"github.com/filecoin-project/go-hamt-ipld/v3"
	"github.com/filecoin-project/go-state-types/abi"
	gstbuiltin "github.com/filecoin-project/go-state-types/builtin"
	datacap16 "github.com/filecoin-project/go-state-types/builtin/v16/datacap"
	market16 "github.com/filecoin-project/go-state-types/builtin/v16/market"
	miner16 "github.com/filecoin-project/go-state-types/builtin/v16/miner"
	power16 "github.com/filecoin-project/go-state-types/builtin/v16/power"
	"github.com/filecoin-project/go-state-types/builtin/v16/util/adt"
	verifreg16 "github.com/filecoin-project/go-state-types/builtin/v16/verifreg"

	"github.com/filecoin-project/lotus/chain/types"
)

// matchKnownBlockType attempts to determine the type of a block by inspecting its bytes. First we
// attempt to decode it as part of a HAMT or AMT, and if we get one, we inspect the types of the
// values. Otherwise we attempt to decode it as a known type using matchKnownBlockTypeFromBytes.
func matchKnownBlockType(ctx context.Context, nd blocks.Block) (string, error) {
	if m, err := matchKnownBlockTypeFromBytes(nd.RawData()); err != nil {
		return "", err
	} else if m != "" {
		return m, nil
	}

	// block store with just one block in it, for interacting with the hamt and amt libraries
	store := cbor.NewMemCborStore()
	if err := store.(*cbor.BasicIpldStore).Blocks.Put(ctx, nd); err != nil {
		return "", err
	}

	// try to load as a HAMT root/node (they are the same thing)
	if _, err := hamt.LoadNode(ctx, store, nd.Cid(), append(adt.DefaultHamtOptions, hamt.UseTreeBitWidth(gstbuiltin.DefaultHamtBitwidth))...); err == nil {
		// got a HAMT, now inspect it
		hamtNode, err := ipld.DecodeUsingPrototype(nd.RawData(), dagcbor.Decode, bindnode.Prototype(nil, knownTypeSystem.TypeByName("HamtNode")))
		if err != nil {
			return "", xerrors.Errorf("failed to decode HamtNode: %w", err)
		}
		typ, err := matchHamtValues(hamtNode)
		if err != nil {
			return "", err
		}
		return fmt.Sprintf("HAMTNode{%d}%s", gstbuiltin.DefaultHamtBitwidth, typ), nil
	}

	// try to load as an AMT root, we have to try all bitwidths used in the chain
	for _, bitwidth := range []uint{2, 3, 4, 5, 6} {
		if _, err := amt.LoadAMT(ctx, store, nd.Cid(), append(adt.DefaultAmtOptions, amt.UseTreeBitWidth(bitwidth))...); err == nil {
			// got an AMT root, now inspect it
			amtRoot, err := ipld.DecodeUsingPrototype(nd.RawData(), dagcbor.Decode, bindnode.Prototype(nil, knownTypeSystem.TypeByName("AMTRoot")))
			if err != nil {
				return "", xerrors.Errorf("failed to decode AMTRoot: %w", err)
			}
			values, err := traversal.Get(amtRoot, datamodel.ParsePath("Node/Values"))
			if err != nil {
				return "", xerrors.Errorf("failed to get AMTRoot.Node.Values: %w", err)
			}
			typ, err := matchAmtValues(values)
			if err != nil {
				return "", err
			}
			return fmt.Sprintf("AMTRoot{%d}%s", bitwidth, typ), nil
		}
	}

	// try to load as an AMT intermediate node, which we can't do using the amt package so we'll
	// infer by schema
	if amtNode, err := ipld.DecodeUsingPrototype(nd.RawData(), dagcbor.Decode, bindnode.Prototype(nil, knownTypeSystem.TypeByName("AMTNode"))); err == nil {
		// got an AMT node, now inspect it
		values, err := amtNode.LookupByString("Values")
		if err != nil {
			return "", xerrors.Errorf("failed to get AMTNode.Values: %w", err)
		}
		typ, err := matchAmtValues(values)
		if err != nil {
			return "", err
		}
		return "AMTNode" + typ, nil
	}

	return "", nil
}

// given a datamodel.Node form of the Values array within an AMT node, attempt to determine the
// type of the values by iterating through them all and checking from their bytes.
func matchAmtValues(values datamodel.Node) (string, error) {
	var match string
	itr := values.ListIterator()
	for !itr.Done() {
		_, v, err := itr.Next()
		if err != nil {
			return "", err
		}
		enc, err := ipld.Encode(v, dagcbor.Encode)
		if err != nil {
			return "", err
		}
		if m, _ := matchKnownBlockTypeFromBytes(enc); m != "" {
			if match == "" {
				match = m
			} else if match != m {
				return "", xerrors.Errorf("inconsistent types in AMT values")
			}
			// To debug unknown AMT types, uncomment this block:
			// } else {
			// 	enc, _ := ipld.Encode(v, dagjson.Encode)
			// 	return "", xerrors.Errorf("unknown type in AMT values: %s", enc)
		}
	}
	if match != "" {
		return "[" + match + "]", nil
	}
	return "", nil
}

// given a datamodel.Node form of a HAMT node, attempt to determine the type of the values, if there
// are any, by iterating through them all and checking from their bytes.
func matchHamtValues(hamtNode datamodel.Node) (string, error) {
	pointers, err := hamtNode.LookupByString("Pointers")
	if err != nil {
		return "", xerrors.Errorf("failed to get HamtNode.Pointers: %w", err)
	}
	var match string
	itr := pointers.ListIterator()
	for !itr.Done() {
		_, v, err := itr.Next()
		if err != nil {
			return "", err
		}
		b, err := v.LookupByString("Bucket")
		if err == nil {
			bitr := b.ListIterator()
			for !bitr.Done() {
				_, kv, err := bitr.Next()
				if err != nil {
					return "", err
				}
				bval, err := kv.LookupByString("Value")
				if err != nil {
					return "", err
				}
				enc, err := ipld.Encode(bval, dagcbor.Encode)
				if err != nil {
					return "", err
				}
				if m, _ := matchKnownBlockTypeFromBytes(enc); m != "" {
					if match == "" {
						match = m
					} else if match != m {
						return "", xerrors.Errorf("inconsistent types in HAMT values: %s != %s", match, m)
					}
					// To debug unknown HAMT types, uncomment this block:
					// } else {
					// 	enc, _ := ipld.Encode(bval, dagjson.Encode)
					// 	return "", xerrors.Errorf("unknown type in HAMT values: %s", enc)
				}
			}
		}
	}
	if match != "" {
		return "[" + match + "]", nil
	}
	return "", nil
}

var wellKnownBlockBytes = map[string][]byte{
	"EmptyArray":          {0x80},
	"EmptyBytes":          {0x40},
	"EmptyString":         {0x60}, // is this used anywhere in the chain?
	"Zero":                {0x00}, // is this used anywhere in the chain?
	"HAMTNode{5}[empty]":  {0x82, 0x40, 0x80},
	"AMTRoot{2/3}[empty]": {0x84, 0x02, 0x00, 0x00, 0x83, 0x41, 0x00, 0x80, 0x80},
	"AMTRoot{4}[empty]":   {0x84, 0x04, 0x00, 0x00, 0x83, 0x42, 0x00, 0x00, 0x80, 0x80},
	"AMTRoot{5}[empty]":   {0x84, 0x05, 0x00, 0x00, 0x83, 0x44, 0x00, 0x00, 0x00, 0x00, 0x80, 0x80},
	"AMTRoot{6}[empty]":   {0x84, 0x06, 0x00, 0x00, 0x83, 0x48, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x80, 0x80},
}

func matchWellKnownBlockType(b []byte) (string, error) {
	for name, wkb := range wellKnownBlockBytes {
		if bytes.Equal(b, wkb) {
			return name, nil
		}
	}
	return "", nil
}

// matchKnownBlockTypeFromBytes attempts to determine the type of a block by inspecting its bytes.
// We use a fixed list of known types that have a CBORUnmarshaler that we believe may be possible.
// This list is not exhaustive and should be expanded as unknown types are encountered.
func matchKnownBlockTypeFromBytes(b []byte) (string, error) {
	if m, _ := matchWellKnownBlockType(b); m != "" {
		return m, nil
	}
	if _, err := cbg.ReadCid(bytes.NewReader(b)); err == nil {
		return "Cid", nil
	}
	ci := cbg.CborInt(1)
	cb := cbg.CborBool(true)
	known := map[string]cbg.CBORUnmarshaler{
		// Fill this out with known types when you see them missing and can identify them
		"BlockHeader":                        &types.BlockHeader{},
		"miner16.State":                      &miner16.State{},
		"miner16.MinerInfo":                  &miner16.MinerInfo{},
		"miner16.Deadlines":                  &miner16.Deadlines{},
		"miner16.Deadline":                   &miner16.Deadline{},
		"miner16.Partition":                  &miner16.Partition{},
		"miner16.ExpirationSet":              &miner16.ExpirationSet{},
		"miner16.WindowedPoSt":               &miner16.WindowedPoSt{},
		"miner16.SectorOnChainInfo":          &miner16.SectorOnChainInfo{},
		"miner16.SectorPreCommitOnChainInfo": &miner16.SectorPreCommitOnChainInfo{},
		"power16.State":                      &power16.State{},
		"market16.State":                     &market16.State{},
		"market16.DealProposal":              &market16.DealProposal{},
		"market16.DealState":                 &market16.DealState{},
		"verifreg16.State":                   &verifreg16.State{},
		"verifreg16.Allocation":              &verifreg16.Allocation{},
		"verifreg16.Claim":                   &verifreg16.Claim{},
		"datacap16.State":                    &datacap16.State{},
		"[Int]":                              &market16.SectorDealIDs{}, // verifreg16.RmDcProposalID is one of these too, as are probably others, we can't be certain, it would be context dependent
		"Int":                                &ci,
		"Bool":                               &cb,
		"Bytes":                              &abi.CborBytes{}, // could be TokenAmount, BigInt, etc
	}
	for name, v := range known {
		if err := v.UnmarshalCBOR(bytes.NewReader(b)); err == nil {
			return name, nil
		}
	}
	return "", nil
}

const knownTypesSchema = `
type HamtNode struct {
	Bitfield Bytes
	Pointers [Pointer]
} representation tuple

type Pointer union {
	| Any link # link to HamtNode
	| Bucket list
} representation kinded

type Bucket [KV]

type KV struct {
	Key Bytes
	Value Any
} representation tuple

type AMTNode struct {
	Bmap   Bytes
	Links  [Link]
	Values [Any]
} representation tuple

type AMTRoot struct {
	BitWidth Int
	Height   Int
	Count    Int
	Node     AMTNode
} representation tuple
`

var knownTypeSystem schema.TypeSystem

func init() {
	sch, err := schemadsl.ParseBytes([]byte(knownTypesSchema))
	if err != nil {
		panic(err)
	}
	knownTypeSystem.Init()
	if err := schemadmt.Compile(&knownTypeSystem, sch); err != nil {
		panic(err)
	}
}

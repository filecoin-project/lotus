package proof

// This file contains some type definitions from
// - https://github.com/filecoin-project/rust-fil-proofs/tree/master/storage-proofs-core/src/merkle
// - https://github.com/filecoin-project/rust-fil-proofs/tree/master/storage-proofs-porep/src/stacked/vanilla
// - https://github.com/filecoin-project/rust-filecoin-proofs-api/tree/master/src

// core

type Commitment [32]byte
type Ticket [32]byte

type StringRegisteredProofType string // e.g. "StackedDrg2KiBV1"

type HasherDomain = any

type Sha256Domain [32]byte

type PoseidonDomain [32]byte // Fr

type MerkleProof[H HasherDomain] struct {
	Data ProofData[H] `json:"data"`
}

type ProofData[H HasherDomain] struct {
	Single *SingleProof[H] `json:"Single,omitempty"`
	Sub    *SubProof[H]    `json:"Sub,omitempty"`
	Top    *TopProof[H]    `json:"Top,omitempty"`
}

type SingleProof[H HasherDomain] struct {
	Root H                `json:"root"`
	Leaf H                `json:"leaf"`
	Path InclusionPath[H] `json:"path"`
}

type SubProof[H HasherDomain] struct {
	BaseProof InclusionPath[H] `json:"base_proof"`
	SubProof  InclusionPath[H] `json:"sub_proof"`
	Root      H                `json:"root"`
	Leaf      H                `json:"leaf"`
}

type TopProof[H HasherDomain] struct {
	BaseProof InclusionPath[H] `json:"base_proof"`
	SubProof  InclusionPath[H] `json:"sub_proof"`
	TopProof  InclusionPath[H] `json:"top_proof"`

	Root H `json:"root"`
	Leaf H `json:"leaf"`
}

type InclusionPath[H HasherDomain] struct {
	Path []PathElement[H] `json:"path"`
}

type PathElement[H HasherDomain] struct {
	Hashes []H    `json:"hashes"`
	Index  uint64 `json:"index"`
}

// porep

type Label struct {
	ID            string `json:"id"`
	Path          string `json:"path"`
	RowsToDiscard int    `json:"rows_to_discard"`
	Size          int    `json:"size"`
}

type Labels struct {
	H      any     `json:"_h"` // todo ?
	Labels []Label `json:"labels"`
}

type PreCommit1OutRaw struct {
	LotusSealRand []byte `json:"_lotus_SealRandomness"`

	CommD           Commitment                           `json:"comm_d"`
	Config          Label                                `json:"config"`
	Labels          map[StringRegisteredProofType]Labels `json:"labels"`
	RegisteredProof StringRegisteredProofType            `json:"registered_proof"`
}

type Commit1OutRaw struct {
	CommD           Commitment                `json:"comm_d"`
	CommR           Commitment                `json:"comm_r"`
	RegisteredProof StringRegisteredProofType `json:"registered_proof"`
	ReplicaID       Commitment                `json:"replica_id"`
	Seed            Ticket                    `json:"seed"`
	Ticket          Ticket                    `json:"ticket"`

	VanillaProofs map[StringRegisteredProofType][][]VanillaStackedProof `json:"vanilla_proofs"`
}

type VanillaStackedProof struct {
	CommDProofs    MerkleProof[Sha256Domain]   `json:"comm_d_proofs"`
	CommRLastProof MerkleProof[PoseidonDomain] `json:"comm_r_last_proof"`

	ReplicaColumnProofs ReplicaColumnProof[PoseidonDomain] `json:"replica_column_proofs"`
	LabelingProofs      []LabelingProof[PoseidonDomain]    `json:"labeling_proofs"`
	EncodingProof       EncodingProof[PoseidonDomain]      `json:"encoding_proof"`
}

type ReplicaColumnProof[H HasherDomain] struct {
	C_X        ColumnProof[H]   `json:"c_x"`
	DrgParents []ColumnProof[H] `json:"drg_parents"`
	ExpParents []ColumnProof[H] `json:"exp_parents"`
}

type ColumnProof[H HasherDomain] struct {
	Column         Column[H]      `json:"column"`
	InclusionProof MerkleProof[H] `json:"inclusion_proof"`
}

type Column[H HasherDomain] struct {
	Index uint32 `json:"index"`
	Rows  []H    `json:"rows"`
	H     any    `json:"_h"`
}

type LabelingProof[H HasherDomain] struct {
	Parents    []H    `json:"parents"`
	LayerIndex uint32 `json:"layer_index"`
	Node       uint64 `json:"node"`
	//H          any    `json:"_h"`
}

type EncodingProof[H HasherDomain] struct {
	Parents    []H    `json:"parents"`
	LayerIndex uint32 `json:"layer_index"`
	Node       uint64 `json:"node"`
	//H          any    `json:"_h"`
}

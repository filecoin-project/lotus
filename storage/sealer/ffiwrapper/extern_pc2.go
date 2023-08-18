package ffiwrapper

import (
	"context"
	"encoding/base64"
	"fmt"
	"os"
	"os/exec"

	"github.com/ipfs/go-cid"
	"golang.org/x/xerrors"

	commcid "github.com/filecoin-project/go-fil-commcid"

	"github.com/filecoin-project/lotus/storage/sealer/commitment"
	"github.com/filecoin-project/lotus/storage/sealer/storiface"
)

// MakeExternPrecommit2 creates an implementation of ExternPrecommit2 backed by
// an external command specified by the command string.
//
// The command will be called with a number of environment variables set:
// * EXTSEAL_PC2_SECTOR_NUM: the sector number
// * EXTSEAL_PC2_SECTOR_MINER: the miner id
// * EXTSEAL_PC2_PROOF_TYPE: the proof type
// * EXTSEAL_PC2_SECTOR_SIZE: the sector size in bytes
// * EXTSEAL_PC2_CACHE: the path to the cache directory
// * EXTSEAL_PC2_SEALED: the path to the sealed sector file (initialized with unsealed data by the caller)
// * EXTSEAL_PC2_PC1OUT: output from rust-fil-proofs precommit1 phase (base64 encoded json)
//
// The command is expected to:
// * Create cache sc-02-data-tree-r* files
// * Create cache sc-02-data-tree-c* files
// * Create cache p_aux / t_aux files
// * Transform the sealed file in place
func MakeExternPrecommit2(command string) ExternPrecommit2 {
	return func(ctx context.Context, sector storiface.SectorRef, cache, sealed string, pc1out storiface.PreCommit1Out) (sealedCID cid.Cid, unsealedCID cid.Cid, err error) {
		ssize, err := sector.ProofType.SectorSize()
		if err != nil {
			return cid.Undef, cid.Undef, err
		}

		// Set environment variables for the external command
		env := []string{
			"EXTSEAL_PC2_SECTOR_NUM=" + sector.ID.Number.String(),
			"EXTSEAL_PC2_SECTOR_MINER=" + sector.ID.Miner.String(),
			"EXTSEAL_PC2_PROOF_TYPE=" + fmt.Sprintf("%d", sector.ProofType),
			"EXTSEAL_PC2_SECTOR_SIZE=" + fmt.Sprintf("%d", ssize),
			"EXTSEAL_PC2_CACHE=" + cache,
			"EXTSEAL_PC2_SEALED=" + sealed,
			"EXTSEAL_PC2_PC1OUT=" + base64.StdEncoding.EncodeToString(pc1out),
		}

		log.Infow("running external sealing call", "method", "precommit2", "command", command, "env", env)

		// Create and run the external command
		cmd := exec.CommandContext(ctx, "sh", "-c", command)
		cmd.Env = env
		cmd.Stdout = os.Stdout
		cmd.Stderr = os.Stderr

		if err := cmd.Run(); err != nil {
			return cid.Cid{}, cid.Cid{}, xerrors.Errorf("external command error: %w", err)
		}

		commr, err := commitment.PAuxCommR(cache)
		if err != nil {
			return cid.Undef, cid.Undef, xerrors.Errorf("reading p_aux: %w", err)
		}

		sealedCID, err = commcid.ReplicaCommitmentV1ToCID(commr[:])
		if err != nil {
			return cid.Cid{}, cid.Cid{}, err
		}

		commd, err := commitment.TreeDCommD(cache)
		if err != nil {
			return cid.Undef, cid.Undef, xerrors.Errorf("reading CommD from tree-d: %w", err)
		}

		unsealedCID, err = commcid.DataCommitmentV1ToCID(commd[:])
		if err != nil {
			return cid.Cid{}, cid.Cid{}, err
		}

		return sealedCID, unsealedCID, nil
	}
}

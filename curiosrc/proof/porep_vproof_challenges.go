package proof

import (
	"encoding/binary"
	"github.com/minio/sha256-simd"
	"math/big"
)

// https://github.com/filecoin-project/rust-fil-proofs/blob/8f5bd86be36a55e33b9b293ba22ea13ca1f28163/storage-proofs-porep/src/stacked/vanilla/challenges.rs#L21

func DeriveInteractiveChallenges(
	challengesPerPartition uint64,
	sectorNnodes SectorNodes,
	ReplicaID Commitment,
	Seed Ticket,
	k uint8,
) []uint64 {
	var jbuf [4]byte

	out := make([]uint64, challengesPerPartition)

	for i := uint64(0); i < challengesPerPartition; i++ {
		// let j: u32 = ((self.challenges_per_partition * k as usize) + i) as u32;
		j := uint32((challengesPerPartition * uint64(k)) + i)

		/*
			let hash = Sha256::new()
				.chain_update(replica_id.into_bytes())
				.chain_update(seed)
				.chain_update(j.to_le_bytes())
				.finalize();

			let bigint = BigUint::from_bytes_le(hash.as_ref());
			bigint_to_challenge(bigint, sector_nodes)
		*/
		hasher := sha256.New()
		hasher.Write(ReplicaID[:])
		hasher.Write(Seed[:])

		binary.LittleEndian.PutUint32(jbuf[:], j)
		hasher.Write(jbuf[:])

		hash := hasher.Sum(nil)
		bigint := new(big.Int).SetBytes(hash) // SetBytes is big-endian

		out[i] = bigintToChallenge(bigint, sectorNnodes)
	}

	return out
}

/*
	fn bigint_to_challenge(bigint: BigUint, sector_nodes: usize) -> usize {
	    debug_assert!(sector_nodes < 1 << 32);
	    // Ensure that we don't challenge the first node.
	    let non_zero_node = (bigint % (sector_nodes - 1)) + 1usize;
	    non_zero_node.to_u32_digits()[0] as usize
	}
*/
func bigintToChallenge(bigint *big.Int, sectorNodes SectorNodes) uint64 {
	// Ensure that we don't challenge the first node.
	nonZeroNode := new(big.Int).Mod(bigint, big.NewInt(int64(sectorNodes-1)))
	nonZeroNode.Add(nonZeroNode, big.NewInt(1))
	return nonZeroNode.Uint64()
}

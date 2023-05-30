package actors

import (
	"github.com/ipfs/go-cid"
	"golang.org/x/xerrors"

	actorstypes "github.com/filecoin-project/go-state-types/actors"
	"github.com/filecoin-project/go-state-types/manifest"
	builtin0 "github.com/filecoin-project/specs-actors/actors/builtin"
	builtin2 "github.com/filecoin-project/specs-actors/v2/actors/builtin"
	builtin3 "github.com/filecoin-project/specs-actors/v3/actors/builtin"
	builtin4 "github.com/filecoin-project/specs-actors/v4/actors/builtin"
	builtin5 "github.com/filecoin-project/specs-actors/v5/actors/builtin"
	builtin6 "github.com/filecoin-project/specs-actors/v6/actors/builtin"
	builtin7 "github.com/filecoin-project/specs-actors/v7/actors/builtin"
)

// GetActorCodeID looks up a builtin actor's code CID by actor version and canonical actor name.
func GetActorCodeID(av actorstypes.Version, name string) (cid.Cid, bool) {

	// Actors V8 and above
	if av >= actorstypes.Version8 {
		if cids, ok := GetActorCodeIDsFromManifest(av); ok {
			c, ok := cids[name]
			return c, ok
		}
	}

	// Actors V7 and lower
	switch name {

	case manifest.AccountKey:
		switch av {

		case actorstypes.Version0:
			return builtin0.AccountActorCodeID, true

		case actorstypes.Version2:
			return builtin2.AccountActorCodeID, true

		case actorstypes.Version3:
			return builtin3.AccountActorCodeID, true

		case actorstypes.Version4:
			return builtin4.AccountActorCodeID, true

		case actorstypes.Version5:
			return builtin5.AccountActorCodeID, true

		case actorstypes.Version6:
			return builtin6.AccountActorCodeID, true

		case actorstypes.Version7:
			return builtin7.AccountActorCodeID, true
		}

	case manifest.CronKey:
		switch av {

		case actorstypes.Version0:
			return builtin0.CronActorCodeID, true

		case actorstypes.Version2:
			return builtin2.CronActorCodeID, true

		case actorstypes.Version3:
			return builtin3.CronActorCodeID, true

		case actorstypes.Version4:
			return builtin4.CronActorCodeID, true

		case actorstypes.Version5:
			return builtin5.CronActorCodeID, true

		case actorstypes.Version6:
			return builtin6.CronActorCodeID, true

		case actorstypes.Version7:
			return builtin7.CronActorCodeID, true
		}

	case manifest.InitKey:
		switch av {

		case actorstypes.Version0:
			return builtin0.InitActorCodeID, true

		case actorstypes.Version2:
			return builtin2.InitActorCodeID, true

		case actorstypes.Version3:
			return builtin3.InitActorCodeID, true

		case actorstypes.Version4:
			return builtin4.InitActorCodeID, true

		case actorstypes.Version5:
			return builtin5.InitActorCodeID, true

		case actorstypes.Version6:
			return builtin6.InitActorCodeID, true

		case actorstypes.Version7:
			return builtin7.InitActorCodeID, true
		}

	case manifest.MarketKey:
		switch av {

		case actorstypes.Version0:
			return builtin0.StorageMarketActorCodeID, true

		case actorstypes.Version2:
			return builtin2.StorageMarketActorCodeID, true

		case actorstypes.Version3:
			return builtin3.StorageMarketActorCodeID, true

		case actorstypes.Version4:
			return builtin4.StorageMarketActorCodeID, true

		case actorstypes.Version5:
			return builtin5.StorageMarketActorCodeID, true

		case actorstypes.Version6:
			return builtin6.StorageMarketActorCodeID, true

		case actorstypes.Version7:
			return builtin7.StorageMarketActorCodeID, true
		}

	case manifest.MinerKey:
		switch av {

		case actorstypes.Version0:
			return builtin0.StorageMinerActorCodeID, true

		case actorstypes.Version2:
			return builtin2.StorageMinerActorCodeID, true

		case actorstypes.Version3:
			return builtin3.StorageMinerActorCodeID, true

		case actorstypes.Version4:
			return builtin4.StorageMinerActorCodeID, true

		case actorstypes.Version5:
			return builtin5.StorageMinerActorCodeID, true

		case actorstypes.Version6:
			return builtin6.StorageMinerActorCodeID, true

		case actorstypes.Version7:
			return builtin7.StorageMinerActorCodeID, true
		}

	case manifest.MultisigKey:
		switch av {

		case actorstypes.Version0:
			return builtin0.MultisigActorCodeID, true

		case actorstypes.Version2:
			return builtin2.MultisigActorCodeID, true

		case actorstypes.Version3:
			return builtin3.MultisigActorCodeID, true

		case actorstypes.Version4:
			return builtin4.MultisigActorCodeID, true

		case actorstypes.Version5:
			return builtin5.MultisigActorCodeID, true

		case actorstypes.Version6:
			return builtin6.MultisigActorCodeID, true

		case actorstypes.Version7:
			return builtin7.MultisigActorCodeID, true
		}

	case manifest.PaychKey:
		switch av {

		case actorstypes.Version0:
			return builtin0.PaymentChannelActorCodeID, true

		case actorstypes.Version2:
			return builtin2.PaymentChannelActorCodeID, true

		case actorstypes.Version3:
			return builtin3.PaymentChannelActorCodeID, true

		case actorstypes.Version4:
			return builtin4.PaymentChannelActorCodeID, true

		case actorstypes.Version5:
			return builtin5.PaymentChannelActorCodeID, true

		case actorstypes.Version6:
			return builtin6.PaymentChannelActorCodeID, true

		case actorstypes.Version7:
			return builtin7.PaymentChannelActorCodeID, true
		}

	case manifest.PowerKey:
		switch av {

		case actorstypes.Version0:
			return builtin0.StoragePowerActorCodeID, true

		case actorstypes.Version2:
			return builtin2.StoragePowerActorCodeID, true

		case actorstypes.Version3:
			return builtin3.StoragePowerActorCodeID, true

		case actorstypes.Version4:
			return builtin4.StoragePowerActorCodeID, true

		case actorstypes.Version5:
			return builtin5.StoragePowerActorCodeID, true

		case actorstypes.Version6:
			return builtin6.StoragePowerActorCodeID, true

		case actorstypes.Version7:
			return builtin7.StoragePowerActorCodeID, true
		}

	case manifest.RewardKey:
		switch av {

		case actorstypes.Version0:
			return builtin0.RewardActorCodeID, true

		case actorstypes.Version2:
			return builtin2.RewardActorCodeID, true

		case actorstypes.Version3:
			return builtin3.RewardActorCodeID, true

		case actorstypes.Version4:
			return builtin4.RewardActorCodeID, true

		case actorstypes.Version5:
			return builtin5.RewardActorCodeID, true

		case actorstypes.Version6:
			return builtin6.RewardActorCodeID, true

		case actorstypes.Version7:
			return builtin7.RewardActorCodeID, true
		}

	case manifest.SystemKey:
		switch av {

		case actorstypes.Version0:
			return builtin0.SystemActorCodeID, true

		case actorstypes.Version2:
			return builtin2.SystemActorCodeID, true

		case actorstypes.Version3:
			return builtin3.SystemActorCodeID, true

		case actorstypes.Version4:
			return builtin4.SystemActorCodeID, true

		case actorstypes.Version5:
			return builtin5.SystemActorCodeID, true

		case actorstypes.Version6:
			return builtin6.SystemActorCodeID, true

		case actorstypes.Version7:
			return builtin7.SystemActorCodeID, true
		}

	case manifest.VerifregKey:
		switch av {

		case actorstypes.Version0:
			return builtin0.VerifiedRegistryActorCodeID, true

		case actorstypes.Version2:
			return builtin2.VerifiedRegistryActorCodeID, true

		case actorstypes.Version3:
			return builtin3.VerifiedRegistryActorCodeID, true

		case actorstypes.Version4:
			return builtin4.VerifiedRegistryActorCodeID, true

		case actorstypes.Version5:
			return builtin5.VerifiedRegistryActorCodeID, true

		case actorstypes.Version6:
			return builtin6.VerifiedRegistryActorCodeID, true

		case actorstypes.Version7:
			return builtin7.VerifiedRegistryActorCodeID, true
		}
	}

	return cid.Undef, false
}

// GetActorCodeIDs looks up all builtin actor's code CIDs by actor version.
func GetActorCodeIDs(av actorstypes.Version) (map[string]cid.Cid, error) {
	cids, ok := GetActorCodeIDsFromManifest(av)
	if ok {
		return cids, nil
	}

	actorsKeys := manifest.GetBuiltinActorsKeys(av)
	synthCids := make(map[string]cid.Cid)

	for _, key := range actorsKeys {
		c, ok := GetActorCodeID(av, key)
		if !ok {
			return nil, xerrors.Errorf("could not find builtin actor cids for Actors version %d", av)
		}
		synthCids[key] = c
	}

	return synthCids, nil
}

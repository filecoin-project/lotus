package main

import (
	"github.com/filecoin-project/go-state-types/abi"

	"github.com/filecoin-project/lotus/build/buildconstants"
)

// ProtocolCodenames is a table that summarises the protocol codenames that
// will be set on extracted vectors, depending on the original execution height.
//
// Implementers rely on these names to filter the vectors they can run through
// their implementations, based on their support level
var ProtocolCodenames = []struct {
	firstEpoch abi.ChainEpoch
	name       string
}{
	{0, "genesis"},
	{buildconstants.UpgradeBreezeHeight + 1, "breeze"},
	{buildconstants.UpgradeSmokeHeight + 1, "smoke"},
	{buildconstants.UpgradeIgnitionHeight + 1, "ignition"},
	{buildconstants.UpgradeRefuelHeight + 1, "refuel"},
	{buildconstants.UpgradeAssemblyHeight + 1, "actorsv2"},
	{buildconstants.UpgradeTapeHeight + 1, "tape"},
	{buildconstants.UpgradeLiftoffHeight + 1, "liftoff"},
	{buildconstants.UpgradeKumquatHeight + 1, "postliftoff"},
	{buildconstants.UpgradeCalicoHeight + 1, "calico"},
	{buildconstants.UpgradePersianHeight + 1, "persian"},
	{buildconstants.UpgradeOrangeHeight + 1, "orange"},
	{buildconstants.UpgradeTrustHeight + 1, "trust"},
	{buildconstants.UpgradeNorwegianHeight + 1, "norwegian"},
	{buildconstants.UpgradeTurboHeight + 1, "turbo"},
	{buildconstants.UpgradeHyperdriveHeight + 1, "hyperdrive"},
	{buildconstants.UpgradeChocolateHeight + 1, "chocolate"},
	{buildconstants.UpgradeOhSnapHeight + 1, "ohsnap"},
}

// GetProtocolCodename gets the protocol codename associated with a height.
func GetProtocolCodename(height abi.ChainEpoch) string {
	for i, v := range ProtocolCodenames {
		if height < v.firstEpoch {
			// found the cutoff, return previous.
			return ProtocolCodenames[i-1].name
		}
	}
	return ProtocolCodenames[len(ProtocolCodenames)-1].name
}

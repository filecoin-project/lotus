package lf3

import "testing"

func TestContractManifest_ParseEthData(t *testing.T) {
	hexReturn := "00000000000000000000000000000000000000000000000000000000004C4B400000000000000000000000000000000000000000000000000000000000000040000000000000000000000000000000000000000000000000000000000000023D8554C172DA3014BCF3150CE78E079B06486FC1D0363321A590B6874E0FB2FC8C35C8922BC90D6926FFDE27D9C27620539F9EB52B7977DF939F07C3E168432A0DA30FC38C700DEFDC8A924652C9BF83D24C0AC4AEDCFAAD6086117E2BB42182DA3D63B7BE90D268A348B92A25CD2D7DEC1E07DE837994EA704F0ABB6194310E54323172E0EA58724699D9C84750088B8AF3FA537B2115AC620FB4E21A110E78200987EEB65816053306E04ECA4342E801C1B0D6111343F36FE50D677B5180301669747AA99FCA24B3EBCFF882AF4BE086E0EBB4C7F2C0024FFF9265E8408AFAB8A881BF568433F3E458EB8A1B861E9D89B021ACC9D1EA233990742B2B916A1FB1D5991326B003A5D490DE81D89BDC99F01FDF42A2244929D1C64A9059B620AE7F6732CF991DB161307993B62B152AB3ED0DC2374968A2F7D131125FEAA6C66D861B504CDAA326E3D7F23E32E17242F0BA1B2D79EA85E653B52E9740999DC846826FFF4FC718B69E6C39BDF67514446D399FFA7A12CC425FBF0FE6A7ADD3209AF97A165CB9EA5723E233E6D219ADBE95BF568B51159C9288411996314A0CAC8E3427620F6D34311A14660BBF2BD0E68115202B5377FA55503B507F409D11A767C43513ACA88A8DE478470DEE22FC62F2D8BA0BBC30BAD0CB4D95ECAAA4235A16381CDA3661256CFCA9BF9CAD693BC0E776F1184D152B8DED5F9565A076756293A895E5F65E187A84FC5FE774734E57BBC6974C53893941EA0ED138799D1F55D873FF83A0EDFFF13A23DF4929BA10A6ED099E509437CEF0BC17E3E065F00F000000"
	resultManifest := `{
  "Pause": false,
  "ProtocolVersion": 5,
  "InitialInstance": 0,
  "BootstrapEpoch": 5000000,
  "NetworkName": "filecoin",
  "ExplicitPower": null,
  "IgnoreECPower": false,
  "InitialPowerTable": null,
  "CommitteeLookback": 10,
  "CatchUpAlignment": 15000000000,
  "Gpbft": {
    "Delta": 6000000000,
    "DeltaBackOffExponent": 2,
    "QualityDeltaMultiplier": 1,
    "MaxLookaheadRounds": 5,
    "ChainProposedLength": 100,
    "RebroadcastBackoffBase": 6000000000,
    "RebroadcastBackoffExponent": 1.3,
    "RebroadcastBackoffSpread": 0.1,
    "RebroadcastBackoffMax": 60000000000
  },
  "EC": {
    "Period": 30000000000,
    "Finality": 900,
    "DelayMultiplier": 2,
    "BaseDecisionBackoffTable": [
      1.3,
      1.69,
      2.2,
      2.86,
      3.71,
      4.83,
      6.27,
      7.5
    ],
    "HeadLookback": 0,
    "Finalize": true
  },
  "CertificateExchange": {
    "ClientRequestTimeout": 10000000000,
    "ServerRequestTimeout": 60000000000,
    "MinimumPollInterval": 30000000000,
    "MaximumPollInterval": 120000000000
  },
  "PubSub": {
    "CompressionEnabled": false
  },
  "ChainExchange": {
    "SubscriptionBufferSize": 32,
    "MaxChainLength": 100,
    "MaxInstanceLookahead": 10,
    "MaxDiscoveredChainsPerInstance": 1000,
    "MaxWantedChainsPerInstance": 1000,
    "RebroadcastInterval": 2000000000,
    "MaxTimestampAge": 8000000000
  }
}
`

	activationEpoch, compressedManifest, err := parseContractReturn(must.One(ethtypes.DecodeHexString(hexReturn)))
	if err != nil {
		t.Fatalf("parseContractReturn failed: %v", err)
	}

	if activationEpoch != 5000000 {
		t.Errorf("expected activationEpoch 5000000, got %d", activationEpoch)
	}

	manifest, err := decompressManifest(compressedManifest)
	if err != nil {
		t.Fatalf("decompressManifest failed: %v", err)
	}

	manifestJSON, err := json.MarshalIndent(manifest, "", "  ")
	if err != nil {
		t.Fatalf("failed to marshal manifest: %v", err)
	}

	if string(manifestJSON) != resultManifest {
		t.Errorf("expected manifest %s, got %s", resultManifest, string(manifestJSON))
	}

}

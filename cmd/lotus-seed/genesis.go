package main

import (
	"encoding/csv"
	"encoding/json"
	"fmt"
	"os"
	"strconv"
	"strings"
	"time"

	"github.com/google/uuid"
	"github.com/mitchellh/go-homedir"
	"github.com/urfave/cli/v2"
	"golang.org/x/xerrors"

	"github.com/filecoin-project/go-address"
	"github.com/filecoin-project/go-state-types/abi"
	"github.com/filecoin-project/go-state-types/big"
	"github.com/filecoin-project/go-state-types/network"

	"github.com/filecoin-project/lotus/blockstore"
	"github.com/filecoin-project/lotus/build/buildconstants"
	"github.com/filecoin-project/lotus/chain/gen"
	genesis2 "github.com/filecoin-project/lotus/chain/gen/genesis"
	proofsffi "github.com/filecoin-project/lotus/chain/proofs/ffi"
	"github.com/filecoin-project/lotus/chain/types"
	"github.com/filecoin-project/lotus/chain/vm"
	"github.com/filecoin-project/lotus/genesis"
	"github.com/filecoin-project/lotus/journal"
	"github.com/filecoin-project/lotus/node/modules/testing"
)

var genesisCmd = &cli.Command{
	Name:        "genesis",
	Description: "manipulate lotus genesis template",
	Subcommands: []*cli.Command{
		genesisNewCmd,
		genesisAddMinerCmd,
		genesisAddMsigsCmd,
		genesisSetVRKCmd,
		genesisSetRemainderCmd,
		genesisSetActorVersionCmd,
		genesisCarCmd,
		genesisSetVRKSignersCmd,
	},
}

var genesisNewCmd = &cli.Command{
	Name:        "new",
	Description: "create new genesis template",
	Flags: []cli.Flag{
		&cli.StringFlag{
			Name: "network-name",
		},
		&cli.TimestampFlag{
			Name:        "timestamp",
			Usage:       "The genesis timestamp specified as a RFC3339 formatted string. Example: 2025-02-27T15:04:05Z",
			DefaultText: "No timestamp",
			Layout:      time.RFC3339,
		},
	},
	Action: func(cctx *cli.Context) error {
		if !cctx.Args().Present() {
			return xerrors.New("seed genesis new [genesis.json]")
		}
		out := genesis.Template{
			NetworkVersion:   buildconstants.GenesisNetworkVersion,
			Accounts:         []genesis.Actor{},
			Miners:           []genesis.Miner{},
			VerifregRootKey:  gen.DefaultVerifregRootkeyActor,
			RemainderAccount: gen.DefaultRemainderAccountActor,
			NetworkName:      cctx.String("network-name"),
		}
		if out.NetworkName == "" {
			out.NetworkName = "localnet-" + uuid.New().String()
		}
		if cctx.IsSet("timestamp") {
			out.Timestamp = uint64(cctx.Timestamp("timestamp").Unix())
		}

		genb, err := json.MarshalIndent(&out, "", "  ")
		if err != nil {
			return err
		}

		genf, err := homedir.Expand(cctx.Args().First())
		if err != nil {
			return err
		}

		if err := os.WriteFile(genf, genb, 0644); err != nil {
			return err
		}

		return nil
	},
}

var genesisAddMinerCmd = &cli.Command{
	Name:        "add-miner",
	Description: "add genesis miner",
	Flags: []cli.Flag{
		&cli.Int64Flag{
			Name:  "balance",
			Value: 50_000_000,
		},
	},
	Action: func(cctx *cli.Context) error {
		if cctx.NArg() != 2 {
			return xerrors.New("seed genesis add-miner [genesis.json] [preseal.json]")
		}

		genf, err := homedir.Expand(cctx.Args().First())
		if err != nil {
			return err
		}

		var template genesis.Template
		genb, err := os.ReadFile(genf)
		if err != nil {
			return xerrors.Errorf("read genesis template: %w", err)
		}

		if err := json.Unmarshal(genb, &template); err != nil {
			return xerrors.Errorf("unmarshal genesis template: %w", err)
		}

		minf, err := homedir.Expand(cctx.Args().Get(1))
		if err != nil {
			return xerrors.Errorf("expand preseal file path: %w", err)
		}
		miners := map[string]genesis.Miner{}
		minb, err := os.ReadFile(minf)
		if err != nil {
			return xerrors.Errorf("read preseal file: %w", err)
		}
		if err := json.Unmarshal(minb, &miners); err != nil {
			return xerrors.Errorf("unmarshal miner info: %w", err)
		}

		balance := cctx.Int64("balance")
		for mn, miner := range miners {
			log.Infof("Adding miner %s to genesis template", mn)
			{
				id := uint64(genesis2.MinerStart) + uint64(len(template.Miners))
				maddr, err := address.NewFromString(mn)
				if err != nil {
					return xerrors.Errorf("parsing miner address: %w", err)
				}
				mid, err := address.IDFromAddress(maddr)
				if err != nil {
					return xerrors.Errorf("getting miner id from address: %w", err)
				}
				if mid != id {
					return xerrors.Errorf("tried to set miner t0%d as t0%d", mid, id)
				}
			}

			template.Miners = append(template.Miners, miner)
			log.Infof("Giving %s some initial balance", miner.Owner)
			template.Accounts = append(template.Accounts, genesis.Actor{
				Type:    genesis.TAccount,
				Balance: big.Mul(big.NewInt(balance), big.NewInt(int64(buildconstants.FilecoinPrecision))),
				Meta:    (&genesis.AccountMeta{Owner: miner.Owner}).ActorMeta(),
			})
		}

		genb, err = json.MarshalIndent(&template, "", "  ")
		if err != nil {
			return err
		}

		if err := os.WriteFile(genf, genb, 0644); err != nil {
			return err
		}

		return nil
	},
}

type GenAccountEntry struct {
	Version       int
	ID            string
	Amount        types.FIL
	VestingMonths int
	CustodianID   int
	M             int
	N             int
	Addresses     []address.Address
	Type          string
	Sig1          string
	Sig2          string
}

var genesisAddMsigsCmd = &cli.Command{
	Name: "add-msigs",
	Action: func(cctx *cli.Context) error {
		if cctx.NArg() < 2 {
			return fmt.Errorf("must specify template file and csv file with accounts")
		}

		genf, err := homedir.Expand(cctx.Args().First())
		if err != nil {
			return err
		}

		csvf, err := homedir.Expand(cctx.Args().Get(1))
		if err != nil {
			return err
		}

		var template genesis.Template
		b, err := os.ReadFile(genf)
		if err != nil {
			return xerrors.Errorf("read genesis template: %w", err)
		}

		if err := json.Unmarshal(b, &template); err != nil {
			return xerrors.Errorf("unmarshal genesis template: %w", err)
		}

		entries, err := parseMultisigCsv(csvf)
		if err != nil {
			return xerrors.Errorf("parsing multisig csv file: %w", err)
		}

		for i, e := range entries {
			if len(e.Addresses) != e.N {
				return fmt.Errorf("entry %d had mismatch between 'N' and number of addresses", i)
			}

			msig := &genesis.MultisigMeta{
				Signers:         e.Addresses,
				Threshold:       e.M,
				VestingDuration: monthsToBlocks(e.VestingMonths),
				VestingStart:    0,
			}

			act := genesis.Actor{
				Type:    genesis.TMultisig,
				Balance: abi.TokenAmount(e.Amount),
				Meta:    msig.ActorMeta(),
			}

			template.Accounts = append(template.Accounts, act)

		}

		b, err = json.MarshalIndent(&template, "", "  ")
		if err != nil {
			return err
		}

		if err := os.WriteFile(genf, b, 0644); err != nil {
			return err
		}
		return nil
	},
}

func monthsToBlocks(nmonths int) int {
	days := uint64((365 * nmonths) / 12)
	return int(days * 24 * 60 * 60 / buildconstants.BlockDelaySecs)
}

func parseMultisigCsv(csvf string) ([]GenAccountEntry, error) {
	fileReader, err := os.Open(csvf)
	if err != nil {
		return nil, xerrors.Errorf("read multisig csv: %w", err)
	}
	defer fileReader.Close() //nolint:errcheck
	r := csv.NewReader(fileReader)
	records, err := r.ReadAll()
	if err != nil {
		return nil, xerrors.Errorf("read multisig csv: %w", err)
	}
	var entries []GenAccountEntry
	for i, e := range records[1:] {
		var addrs []address.Address
		addrStrs := strings.Split(strings.TrimSpace(e[7]), ":")
		for j, a := range addrStrs {
			addr, err := address.NewFromString(a)
			if err != nil {
				return nil, xerrors.Errorf("failed to parse address %d in row %d (%q): %w", j, i, a, err)
			}
			addrs = append(addrs, addr)
		}

		balance, err := types.ParseFIL(strings.TrimSpace(e[2]))
		if err != nil {
			return nil, xerrors.Errorf("failed to parse account balance: %w", err)
		}

		vesting, err := strconv.Atoi(strings.TrimSpace(e[3]))
		if err != nil {
			return nil, xerrors.Errorf("failed to parse vesting duration for record %d: %w", i, err)
		}

		custodianID, err := strconv.Atoi(strings.TrimSpace(e[4]))
		if err != nil {
			return nil, xerrors.Errorf("failed to parse custodianID in record %d: %w", i, err)
		}
		threshold, err := strconv.Atoi(strings.TrimSpace(e[5]))
		if err != nil {
			return nil, xerrors.Errorf("failed to parse multisigM in record %d: %w", i, err)
		}
		num, err := strconv.Atoi(strings.TrimSpace(e[6]))
		if err != nil {
			return nil, xerrors.Errorf("Number of addresses be integer: %w", err)
		}
		if e[0] != "1" {
			return nil, xerrors.Errorf("record version must be 1")
		}
		entries = append(entries, GenAccountEntry{
			Version:       1,
			ID:            e[1],
			Amount:        balance,
			CustodianID:   custodianID,
			VestingMonths: vesting,
			M:             threshold,
			N:             num,
			Type:          e[8],
			Sig1:          e[9],
			Sig2:          e[10],
			Addresses:     addrs,
		})
	}

	return entries, nil
}

var genesisSetVRKCmd = &cli.Command{
	Name:  "set-vrk",
	Usage: "Set the verified registry's root key",
	Flags: []cli.Flag{
		&cli.StringFlag{
			Name:  "multisig",
			Usage: "CSV file to parse the multisig that will be set as the root key",
		},
		&cli.StringFlag{
			Name:  "account",
			Usage: "pubkey address that will be set as the root key (must NOT be declared anywhere else, since it must be given ID 80)",
		},
	},
	Action: func(cctx *cli.Context) error {
		if cctx.NArg() != 1 {
			return fmt.Errorf("must specify template file")
		}

		genf, err := homedir.Expand(cctx.Args().First())
		if err != nil {
			return err
		}

		var template genesis.Template
		b, err := os.ReadFile(genf)
		if err != nil {
			return xerrors.Errorf("read genesis template: %w", err)
		}

		if err := json.Unmarshal(b, &template); err != nil {
			return xerrors.Errorf("unmarshal genesis template: %w", err)
		}

		if cctx.IsSet("account") {
			addr, err := address.NewFromString(cctx.String("account"))
			if err != nil {
				return err
			}

			am := genesis.AccountMeta{Owner: addr}

			template.VerifregRootKey = genesis.Actor{
				Type:    genesis.TAccount,
				Balance: big.Zero(),
				Meta:    am.ActorMeta(),
			}
		} else if cctx.IsSet("multisig") {
			csvf, err := homedir.Expand(cctx.String("multisig"))
			if err != nil {
				return err
			}

			entries, err := parseMultisigCsv(csvf)
			if err != nil {
				return xerrors.Errorf("parsing multisig csv file: %w", err)
			}

			if len(entries) == 0 {
				return xerrors.Errorf("no msig entries in csv file: %w", err)
			}

			e := entries[0]
			if len(e.Addresses) != e.N {
				return fmt.Errorf("entry had mismatch between 'N' and number of addresses")
			}

			msig := &genesis.MultisigMeta{
				Signers:         e.Addresses,
				Threshold:       e.M,
				VestingDuration: monthsToBlocks(e.VestingMonths),
				VestingStart:    0,
			}

			act := genesis.Actor{
				Type:    genesis.TMultisig,
				Balance: abi.TokenAmount(e.Amount),
				Meta:    msig.ActorMeta(),
			}

			template.VerifregRootKey = act
		} else {
			return xerrors.Errorf("must include either --account or --multisig flag")
		}

		b, err = json.MarshalIndent(&template, "", "  ")
		if err != nil {
			return err
		}

		if err := os.WriteFile(genf, b, 0644); err != nil {
			return err
		}
		return nil
	},
}

var genesisSetRemainderCmd = &cli.Command{
	Name:  "set-remainder",
	Usage: "Set the remainder actor",
	Flags: []cli.Flag{
		&cli.StringFlag{
			Name:  "multisig",
			Usage: "CSV file to parse the multisig that will be set as the remainder actor",
		},
		&cli.StringFlag{
			Name:  "account",
			Usage: "pubkey address that will be set as the remainder key (must NOT be declared anywhere else, since it must be given ID 90)",
		},
	},
	Action: func(cctx *cli.Context) error {
		if cctx.NArg() != 1 {
			return fmt.Errorf("must specify template file")
		}

		genf, err := homedir.Expand(cctx.Args().First())
		if err != nil {
			return err
		}

		var template genesis.Template
		b, err := os.ReadFile(genf)
		if err != nil {
			return xerrors.Errorf("read genesis template: %w", err)
		}

		if err := json.Unmarshal(b, &template); err != nil {
			return xerrors.Errorf("unmarshal genesis template: %w", err)
		}

		if cctx.IsSet("account") {
			addr, err := address.NewFromString(cctx.String("account"))
			if err != nil {
				return err
			}

			am := genesis.AccountMeta{Owner: addr}

			template.RemainderAccount = genesis.Actor{
				Type:    genesis.TAccount,
				Balance: big.Zero(),
				Meta:    am.ActorMeta(),
			}
		} else if cctx.IsSet("multisig") {
			csvf, err := homedir.Expand(cctx.String("multisig"))
			if err != nil {
				return err
			}

			entries, err := parseMultisigCsv(csvf)
			if err != nil {
				return xerrors.Errorf("parsing multisig csv file: %w", err)
			}

			if len(entries) == 0 {
				return xerrors.Errorf("no msig entries in csv file: %w", err)
			}

			e := entries[0]
			if len(e.Addresses) != e.N {
				return fmt.Errorf("entry had mismatch between 'N' and number of addresses")
			}

			msig := &genesis.MultisigMeta{
				Signers:         e.Addresses,
				Threshold:       e.M,
				VestingDuration: monthsToBlocks(e.VestingMonths),
				VestingStart:    0,
			}

			act := genesis.Actor{
				Type:    genesis.TMultisig,
				Balance: abi.TokenAmount(e.Amount),
				Meta:    msig.ActorMeta(),
			}

			template.RemainderAccount = act
		} else {
			return xerrors.Errorf("must include either --account or --multisig flag")
		}

		b, err = json.MarshalIndent(&template, "", "  ")
		if err != nil {
			return err
		}

		if err := os.WriteFile(genf, b, 0644); err != nil {
			return err
		}
		return nil
	},
}

var genesisSetActorVersionCmd = &cli.Command{
	Name:  "set-network-version",
	Usage: "Set the version that this network will start from",
	Flags: []cli.Flag{
		&cli.IntFlag{
			Name:  "network-version",
			Usage: "network version to start genesis with",
			Value: int(buildconstants.GenesisNetworkVersion),
		},
	},
	ArgsUsage: "<genesisFile>",
	Action: func(cctx *cli.Context) error {
		if cctx.NArg() != 1 {
			return fmt.Errorf("must specify genesis file")
		}

		genf, err := homedir.Expand(cctx.Args().First())
		if err != nil {
			return err
		}

		var template genesis.Template
		b, err := os.ReadFile(genf)
		if err != nil {
			return xerrors.Errorf("read genesis template: %w", err)
		}

		if err := json.Unmarshal(b, &template); err != nil {
			return xerrors.Errorf("unmarshal genesis template: %w", err)
		}

		nv := network.Version(cctx.Int("network-version"))
		if nv > buildconstants.TestNetworkVersion {
			return xerrors.Errorf("invalid network version: %d", nv)
		}

		template.NetworkVersion = nv

		b, err = json.MarshalIndent(&template, "", "  ")
		if err != nil {
			return err
		}

		if err := os.WriteFile(genf, b, 0644); err != nil {
			return err
		}
		return nil
	},
}

var genesisCarCmd = &cli.Command{
	Name:        "car",
	Description: "write genesis car file",
	ArgsUsage:   "genesis template `FILE`",
	Flags: []cli.Flag{
		&cli.StringFlag{
			Name:    "out",
			Aliases: []string{"o"},
			Value:   "genesis.car",
			Usage:   "write output to `FILE`",
		},
	},
	Action: func(c *cli.Context) error {
		if c.Args().Len() != 1 {
			return xerrors.Errorf("Please specify a genesis template. (i.e, the one created with `genesis new`)")
		}
		ofile := c.String("out")
		jrnl := journal.NilJournal()
		bstor := blockstore.WrapIDStore(blockstore.NewMemorySync())
		sbldr := vm.Syscalls(proofsffi.ProofVerifier)

		_, err := testing.MakeGenesis(ofile, c.Args().First())(bstor, sbldr, jrnl)()
		return err
	},
}

var genesisSetVRKSignersCmd = &cli.Command{
	Name:  "set-signers",
	Usage: "",
	Flags: []cli.Flag{
		&cli.IntFlag{
			Name:  "threshold",
			Usage: "change the verifreg signer threshold",
		},
		&cli.StringSliceFlag{
			Name:  "signers",
			Usage: "verifreg signers",
		},
	},
	Action: func(cctx *cli.Context) error {
		if cctx.NArg() != 1 {
			return fmt.Errorf("must specify template file")
		}

		genf, err := homedir.Expand(cctx.Args().First())
		if err != nil {
			return err
		}

		var template genesis.Template
		b, err := os.ReadFile(genf)
		if err != nil {
			return xerrors.Errorf("read genesis template: %w", err)
		}

		if err := json.Unmarshal(b, &template); err != nil {
			return xerrors.Errorf("unmarshal genesis template: %w", err)
		}

		var signers []address.Address
		var rootkeyMultisig genesis.MultisigMeta
		if cctx.IsSet("signers") {
			for _, s := range cctx.StringSlice("signers") {
				signer, err := address.NewFromString(s)
				if err != nil {
					return err
				}
				signers = append(signers, signer)
				template.Accounts = append(template.Accounts, genesis.Actor{
					Type:    genesis.TAccount,
					Balance: big.Mul(big.NewInt(50_000), big.NewInt(int64(buildconstants.FilecoinPrecision))),
					Meta:    (&genesis.AccountMeta{Owner: signer}).ActorMeta(),
				})
			}
			rootkeyMultisig = genesis.MultisigMeta{
				Signers:         signers,
				Threshold:       1,
				VestingDuration: 0,
				VestingStart:    0,
			}

		}

		if cctx.IsSet("threshold") {
			rootkeyMultisig = genesis.MultisigMeta{
				Signers:         signers,
				Threshold:       cctx.Int("threshold"),
				VestingDuration: 0,
				VestingStart:    0,
			}
		}

		newVrk := genesis.Actor{
			Type:    genesis.TMultisig,
			Balance: big.NewInt(0),
			Meta:    rootkeyMultisig.ActorMeta(),
		}

		template.VerifregRootKey = newVrk

		b, err = json.MarshalIndent(&template, "", "  ")
		if err != nil {
			return err
		}

		if err := os.WriteFile(genf, b, 0644); err != nil {
			return err
		}
		return nil
	},
}

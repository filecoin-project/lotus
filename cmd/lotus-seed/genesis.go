package main

import (
	"encoding/csv"
	"encoding/json"
	"fmt"
	"io/ioutil"
	"os"
	"strconv"
	"strings"

	"github.com/docker/go-units"
	"github.com/filecoin-project/lotus/lib/tablewriter"
	"github.com/filecoin-project/sector-storage/ffiwrapper"

	"github.com/google/uuid"
	"github.com/mitchellh/go-homedir"
	"github.com/urfave/cli/v2"
	"golang.org/x/xerrors"

	"github.com/filecoin-project/go-address"
	"github.com/filecoin-project/specs-actors/actors/abi"
	"github.com/filecoin-project/specs-actors/actors/abi/big"

	"github.com/filecoin-project/lotus/build"
	"github.com/filecoin-project/lotus/chain/gen"
	genesis2 "github.com/filecoin-project/lotus/chain/gen/genesis"
	"github.com/filecoin-project/lotus/chain/types"
	"github.com/filecoin-project/lotus/genesis"
)

var genesisCmd = &cli.Command{
	Name:        "genesis",
	Description: "manipulate lotus genesis template",
	Subcommands: []*cli.Command{
		genesisNewCmd,
		genesisAddMinerCmd,
		genesisAddMsigsCmd,
		genesisAccountInfo,
		genesisBindAccountMinerCmd,
	},
}

var genesisNewCmd = &cli.Command{
	Name:        "new",
	Description: "create new genesis template",
	Flags: []cli.Flag{
		&cli.StringFlag{
			Name: "network-name",
		},
	},
	Action: func(cctx *cli.Context) error {
		if !cctx.Args().Present() {
			return xerrors.New("seed genesis new [genesis.json]")
		}
		out := genesis.Template{
			Accounts:        []genesis.Actor{},
			Miners:          []genesis.Miner{},
			VerifregRootKey: gen.DefaultVerifregRootkeyActor,
			InitIDStart:     genesis2.MinerStart,
			NetworkName:     cctx.String("network-name"),
		}
		if out.NetworkName == "" {
			out.NetworkName = "localnet-" + uuid.New().String()
		}

		genb, err := json.MarshalIndent(&out, "", "  ")
		if err != nil {
			return err
		}

		genf, err := homedir.Expand(cctx.Args().First())
		if err != nil {
			return err
		}

		if err := ioutil.WriteFile(genf, genb, 0644); err != nil {
			return err
		}

		return nil
	},
}

var genesisAddMinerCmd = &cli.Command{
	Name:        "add-miner",
	Description: "add genesis miner",
	Flags:       []cli.Flag{},
	Action: func(cctx *cli.Context) error {
		if cctx.Args().Len() != 2 {
			return xerrors.New("seed genesis add-miner [genesis.json] [preseal.json]")
		}

		genf, err := homedir.Expand(cctx.Args().First())
		if err != nil {
			return err
		}

		var template genesis.Template
		genb, err := ioutil.ReadFile(genf)
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
		minb, err := ioutil.ReadFile(minf)
		if err != nil {
			return xerrors.Errorf("read preseal file: %w", err)
		}
		if err := json.Unmarshal(minb, &miners); err != nil {
			return xerrors.Errorf("unmarshal miner info: %w", err)
		}

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
				Balance: big.Mul(big.NewInt(50_000_000), big.NewInt(int64(build.FilecoinPrecision))),
				Meta:    (&genesis.AccountMeta{Owner: miner.Owner}).ActorMeta(),
			})
		}

		genb, err = json.MarshalIndent(&template, "", "  ")
		if err != nil {
			return err
		}

		if err := ioutil.WriteFile(genf, genb, 0644); err != nil {
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
		if cctx.Args().Len() < 2 {
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
		b, err := ioutil.ReadFile(genf)
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

		if err := ioutil.WriteFile(genf, b, 0644); err != nil {
			return err
		}
		return nil
	},
}

func monthsToBlocks(nmonths int) int {
	days := uint64((365 * nmonths) / 12)
	return int(days * 24 * 60 * 60 / build.BlockDelaySecs)
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

var genesisAccountInfo = &cli.Command{
	Name:        "info",
	Description: "list genesis account info",
	Usage:       "[genesis.json]",
	Action: func(cctx *cli.Context) error {
		if cctx.Args().Len() != 1 {
			return xerrors.New("seed genesis info [genesis.json]")
		}
		genf, err := homedir.Expand(cctx.Args().First())
		if err != nil {
			return err
		}
		var template genesis.Template
		genb, err := ioutil.ReadFile(genf)
		if err != nil {
			return xerrors.Errorf("read genesis template: %w", err)
		}

		if err := json.Unmarshal(genb, &template); err != nil {
			return xerrors.Errorf("unmarshal genesis template: %w", err)
		}
		return printAccounts(template)
	},
}

var genesisBindAccountMinerCmd = &cli.Command{
	Name:        "bind-miner",
	Description: "genesis bind account and  miner",
	Usage:       "[genesis.json] [wallet address] [storage miner address]",
	Flags: []cli.Flag{
		&cli.StringFlag{
			Name:  "sector-size",
			Value: "32GiB",
			Usage: "specify size of sectors to bind miner",
		},
	},
	Action: func(cctx *cli.Context) error {
		if cctx.Args().Len() != 3 {
			return xerrors.New("seed genesis bind-miner [genesis.json] [wallet address] [storage miner address]")
		}
		genf, err := homedir.Expand(cctx.Args().First())
		if err != nil {
			return err
		}
		var template genesis.Template
		genb, err := ioutil.ReadFile(genf)
		if err != nil {
			return xerrors.Errorf("read genesis template: %w", err)
		}

		if err := json.Unmarshal(genb, &template); err != nil {
			return xerrors.Errorf("unmarshal genesis template: %w", err)
		}
		walletAddr, err := address.NewFromString(cctx.Args().Get(1))
		if err != nil {
			return err
		}
		minerAddr, err := address.NewFromString(cctx.Args().Get(2))
		if err != nil {
			return err
		}
		sectorSizeInt, err := units.RAMInBytes(cctx.String("sector-size"))
		if err != nil {
			return err
		}
		sectorSize := abi.SectorSize(sectorSizeInt)
		spt, err := ffiwrapper.SealProofTypeFromSectorSize(sectorSize)
		if err != nil {
			return err
		}

		fmt.Printf("bind before:\n")
		_ = printAccounts(template)

		isExistAccountActor := false
		for index, account := range template.Accounts {
			meta, err := genesis2.GetAccountMeta(account)
			if err != nil {
				return err
			}
			//check template account ids
			if addressIndexOf(account.BindMiners, minerAddr) {
				return xerrors.Errorf("miner address %v already bind with account %v", meta.Owner, minerAddr)
			}

			if meta.Owner == walletAddr {
				isExistAccountActor = true
				account.BindMiners = append(account.BindMiners, genesis.BindMiner{
					Address:   minerAddr,
					SealProof: spt,
				})
				minBalance := abi.NewTokenAmount(int64(len(account.BindMiners)))
				if types.BigCmp(account.Balance, minBalance) == -1 {
					template.Accounts[index].Balance = minBalance
					fmt.Printf("Giving %s some balance:%v\n", walletAddr, types.FIL(account.Balance))
				}
				template.Accounts[index].BindMiners = account.BindMiners
			}
		}

		if !isExistAccountActor {
			act := genesis.Actor{
				Type:       genesis.TAccount,
				Balance:    abi.NewTokenAmount(1),
				BindMiners: []genesis.BindMiner{{Address: minerAddr, SealProof: spt}},
				Meta:       (&genesis.AccountMeta{Owner: walletAddr}).ActorMeta(),
			}
			fmt.Printf("Adding and Giving %s some balance:%v\n", walletAddr, types.FIL(act.Balance))
			template.Accounts = append(template.Accounts, act)
		}

		maxMinerActorID, err := getMaxMinerID(template)
		if err != nil {
			return err
		}
		if maxMinerActorID > genesis2.MinerStart {
			template.InitIDStart = maxMinerActorID + 1
		} else {
			template.InitIDStart = genesis2.MinerStart
		}
		fmt.Printf("bind after:\n")
		_ = printAccounts(template)
		genb, err = json.MarshalIndent(&template, "", "  ")
		if err != nil {
			return err
		}

		if err := ioutil.WriteFile(genf, genb, 0600); err != nil {
			return err
		}

		return nil
	},
}

func getMaxMinerID(template genesis.Template) (uint64, error) {
	max := uint64(len(template.Miners))
	for _, account := range template.Accounts {
		for _, m := range account.BindMiners {
			id, err := address.IDFromAddress(m.Address)
			if err != nil {
				return 0, err
			}
			if id > max {
				max = id
			}
		}
	}
	return max, nil
}

func printAccounts(template genesis.Template) error {

	w := tablewriter.New(
		tablewriter.Col("Owner"),
		tablewriter.Col("Type"),
		tablewriter.Col("Balance"),
		tablewriter.Col("Miners"),
	)
	for _, account := range template.Accounts {
		meta, err := genesis2.GetAccountMeta(account)
		if err != nil {
			return err
		}
		w.Write(map[string]interface{}{
			"Owner":   meta.Owner,
			"Type":    account.Type,
			"Balance": types.FIL(account.Balance),
			"Miners":  account.BindMiners,
		})
	}
	return w.Flush(os.Stdout)
}

func addressIndexOf(addrs []genesis.BindMiner, addr address.Address) bool {
	for _, v := range addrs {
		if v.Address == addr {
			return true
		}
	}
	return false
}

package main

import (
	"bufio"
	"bytes"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"

	"github.com/docker/go-units"
	"github.com/ipfs/go-cid"
	"github.com/mitchellh/go-homedir"
	"github.com/urfave/cli/v2"
	"golang.org/x/xerrors"

	"github.com/filecoin-project/go-address"
	"github.com/filecoin-project/go-bitfield"
	"github.com/filecoin-project/go-state-types/abi"
	"github.com/filecoin-project/go-state-types/big"
	"github.com/filecoin-project/go-state-types/builtin"
	miner8 "github.com/filecoin-project/go-state-types/builtin/v8/miner"
	"github.com/filecoin-project/go-state-types/crypto"
	power7 "github.com/filecoin-project/specs-actors/v7/actors/builtin/power"
	"github.com/filecoin-project/specs-actors/v7/actors/runtime/proof"

	"github.com/filecoin-project/lotus/build"
	"github.com/filecoin-project/lotus/chain/actors"
	"github.com/filecoin-project/lotus/chain/actors/builtin/miner"
	"github.com/filecoin-project/lotus/chain/actors/builtin/power"
	"github.com/filecoin-project/lotus/chain/types"
	lcli "github.com/filecoin-project/lotus/cli"
)

var minerCmd = &cli.Command{
	Name:  "miner",
	Usage: "miner-related utilities",
	Subcommands: []*cli.Command{
		minerUnpackInfoCmd,
		minerCreateCmd,
		minerFaultsCmd,
		sendInvalidWindowPoStCmd,
		generateAndSendConsensusFaultCmd,
	},
}

var minerFaultsCmd = &cli.Command{
	Name:      "faults",
	Usage:     "Display a list of faulty sectors for a SP",
	ArgsUsage: "[minerAddress]",
	Flags: []cli.Flag{
		&cli.Uint64Flag{
			Name:  "expiring-in",
			Usage: "only list sectors that are expiring in the next <n> epochs",
			Value: 0,
		},
	},
	Action: func(cctx *cli.Context) error {
		if !cctx.Args().Present() {
			return fmt.Errorf("must pass miner address")
		}

		api, closer, err := lcli.GetFullNodeAPI(cctx)
		if err != nil {
			return err
		}

		defer closer()

		ctx := lcli.ReqContext(cctx)

		m, err := address.NewFromString(cctx.Args().First())
		if err != nil {
			return err
		}

		faultBf, err := api.StateMinerFaults(ctx, m, types.EmptyTSK)
		if err != nil {
			return err
		}

		faults, err := faultBf.All(abi.MaxSectorNumber)
		if err != nil {
			return err
		}

		if len(faults) == 0 {
			fmt.Println("no faults")
			return nil
		}

		expEpoch := abi.ChainEpoch(cctx.Uint64("expiring-in"))

		if expEpoch == 0 {
			fmt.Print("faulty sectors: ")
			for _, v := range faults {
				fmt.Printf("%d ", v)
			}

			return nil
		}

		h, err := api.ChainHead(ctx)
		if err != nil {
			return err
		}

		fmt.Printf("faulty sectors expiring in the next %d epochs: ", expEpoch)
		for _, v := range faults {
			ss, err := api.StateSectorExpiration(ctx, m, abi.SectorNumber(v), types.EmptyTSK)
			if err != nil {
				return err
			}

			if ss.Early < h.Height()+expEpoch {
				fmt.Printf("%d ", v)
			}
		}

		return nil
	},
}

var minerCreateCmd = &cli.Command{
	Name:      "create",
	Usage:     "sends a create miner message",
	ArgsUsage: "[sender] [owner] [worker] [sector size]",
	Action: func(cctx *cli.Context) error {
		wapi, closer, err := lcli.GetFullNodeAPI(cctx)
		if err != nil {
			return err
		}

		defer closer()
		ctx := lcli.ReqContext(cctx)

		if cctx.NArg() != 4 {
			return lcli.IncorrectNumArgs(cctx)
		}

		sender, err := address.NewFromString(cctx.Args().First())
		if err != nil {
			return err
		}

		owner, err := address.NewFromString(cctx.Args().Get(1))
		if err != nil {
			return err
		}

		worker, err := address.NewFromString(cctx.Args().Get(2))
		if err != nil {
			return err
		}

		ssize, err := units.RAMInBytes(cctx.Args().Get(3))
		if err != nil {
			return fmt.Errorf("failed to parse sector size: %w", err)
		}

		// make sure the sender account exists on chain
		_, err = wapi.StateLookupID(ctx, owner, types.EmptyTSK)
		if err != nil {
			return xerrors.Errorf("sender must exist on chain: %w", err)
		}

		// make sure the worker account exists on chain
		_, err = wapi.StateLookupID(ctx, worker, types.EmptyTSK)
		if err != nil {
			signed, err := wapi.MpoolPushMessage(ctx, &types.Message{
				From:  sender,
				To:    worker,
				Value: types.NewInt(0),
			}, nil)
			if err != nil {
				return xerrors.Errorf("push worker init: %w", err)
			}

			log.Infof("Initializing worker account %s, message: %s", worker, signed.Cid())
			log.Infof("Waiting for confirmation")

			mw, err := wapi.StateWaitMsg(ctx, signed.Cid(), build.MessageConfidence)
			if err != nil {
				return xerrors.Errorf("waiting for worker init: %w", err)
			}

			if mw.Receipt.ExitCode != 0 {
				return xerrors.Errorf("initializing worker account failed: exit code %d", mw.Receipt.ExitCode)
			}
		}

		// make sure the owner account exists on chain
		_, err = wapi.StateLookupID(ctx, owner, types.EmptyTSK)
		if err != nil {
			signed, err := wapi.MpoolPushMessage(ctx, &types.Message{
				From:  sender,
				To:    owner,
				Value: types.NewInt(0),
			}, nil)
			if err != nil {
				return xerrors.Errorf("push owner init: %w", err)
			}

			log.Infof("Initializing owner account %s, message: %s", worker, signed.Cid())
			log.Infof("Wating for confirmation")

			mw, err := wapi.StateWaitMsg(ctx, signed.Cid(), build.MessageConfidence)
			if err != nil {
				return xerrors.Errorf("waiting for owner init: %w", err)
			}

			if mw.Receipt.ExitCode != 0 {
				return xerrors.Errorf("initializing owner account failed: exit code %d", mw.Receipt.ExitCode)
			}
		}

		// Note: the correct thing to do would be to call SealProofTypeFromSectorSize if actors version is v3 or later, but this still works
		spt, err := miner.WindowPoStProofTypeFromSectorSize(abi.SectorSize(ssize))
		if err != nil {
			return xerrors.Errorf("getting post proof type: %w", err)
		}

		params, err := actors.SerializeParams(&power7.CreateMinerParams{
			Owner:               owner,
			Worker:              worker,
			WindowPoStProofType: spt,
		})

		if err != nil {
			return err
		}

		createStorageMinerMsg := &types.Message{
			To:    power.Address,
			From:  sender,
			Value: big.Zero(),

			Method: power.Methods.CreateMiner,
			Params: params,
		}

		signed, err := wapi.MpoolPushMessage(ctx, createStorageMinerMsg, nil)
		if err != nil {
			return xerrors.Errorf("pushing createMiner message: %w", err)
		}

		log.Infof("Pushed CreateMiner message: %s", signed.Cid())
		log.Infof("Waiting for confirmation")

		mw, err := wapi.StateWaitMsg(ctx, signed.Cid(), build.MessageConfidence)
		if err != nil {
			return xerrors.Errorf("waiting for createMiner message: %w", err)
		}

		if mw.Receipt.ExitCode != 0 {
			return xerrors.Errorf("create miner failed: exit code %d", mw.Receipt.ExitCode)
		}

		var retval power7.CreateMinerReturn
		if err := retval.UnmarshalCBOR(bytes.NewReader(mw.Receipt.Return)); err != nil {
			return err
		}

		log.Infof("New miners address is: %s (%s)", retval.IDAddress, retval.RobustAddress)

		return nil
	},
}

var minerUnpackInfoCmd = &cli.Command{
	Name:      "unpack-info",
	Usage:     "unpack miner info all dump",
	ArgsUsage: "[allinfo.txt] [dir]",
	Action: func(cctx *cli.Context) error {
		if cctx.NArg() != 2 {
			return lcli.IncorrectNumArgs(cctx)
		}

		src, err := homedir.Expand(cctx.Args().Get(0))
		if err != nil {
			return xerrors.Errorf("expand src: %w", err)
		}

		f, err := os.Open(src)
		if err != nil {
			return xerrors.Errorf("open file: %w", err)
		}
		defer f.Close() // nolint

		dest, err := homedir.Expand(cctx.Args().Get(1))
		if err != nil {
			return xerrors.Errorf("expand dest: %w", err)
		}

		var outf *os.File

		r := bufio.NewReader(f)
		for {
			l, _, err := r.ReadLine()
			if err == io.EOF {
				if outf != nil {
					return outf.Close()
				}
			}
			if err != nil {
				return xerrors.Errorf("read line: %w", err)
			}
			sl := string(l)

			if strings.HasPrefix(sl, "#") {
				if strings.Contains(sl, "..") {
					return xerrors.Errorf("bad name %s", sl)
				}

				if strings.HasPrefix(sl, "#: ") {
					if outf != nil {
						if err := outf.Close(); err != nil {
							return xerrors.Errorf("close out file: %w", err)
						}
					}
					p := filepath.Join(dest, sl[len("#: "):])
					if err := os.MkdirAll(filepath.Dir(p), 0775); err != nil {
						return xerrors.Errorf("mkdir: %w", err)
					}
					outf, err = os.Create(p)
					if err != nil {
						return xerrors.Errorf("create out file: %w", err)
					}
					continue
				}

				if strings.HasPrefix(sl, "##: ") {
					if outf != nil {
						if err := outf.Close(); err != nil {
							return xerrors.Errorf("close out file: %w", err)
						}
					}
					p := filepath.Join(dest, "Per Sector Infos", sl[len("##: "):])
					if err := os.MkdirAll(filepath.Dir(p), 0775); err != nil {
						return xerrors.Errorf("mkdir: %w", err)
					}
					outf, err = os.Create(p)
					if err != nil {
						return xerrors.Errorf("create out file: %w", err)
					}
					continue
				}
			}

			if outf != nil {
				if _, err := outf.Write(l); err != nil {
					return xerrors.Errorf("write line: %w", err)
				}
				if _, err := outf.Write([]byte("\n")); err != nil {
					return xerrors.Errorf("write line end: %w", err)
				}
			}
		}
	},
}

var sendInvalidWindowPoStCmd = &cli.Command{
	Name:        "send-invalid-windowed-post",
	Usage:       "Sends an invalid windowed post for a specific deadline",
	Description: `Note: This is meant for testing purposes and should NOT be used on mainnet or you will be slashed`,
	Flags: []cli.Flag{
		&cli.BoolFlag{
			Name:  "really-do-it",
			Usage: "Actually send transaction performing the action",
			Value: false,
		},
		&cli.Int64SliceFlag{
			Name:     "partitions",
			Usage:    "list of partitions to submit invalid post for",
			Required: true,
		},
		&cli.StringFlag{
			Name:  "actor",
			Usage: "Specify the address of the miner to run this command",
		},
	},
	Action: func(cctx *cli.Context) error {
		if !cctx.Bool("really-do-it") {
			return xerrors.Errorf("Pass --really-do-it to actually execute this action")
		}

		api, acloser, err := lcli.GetFullNodeAPI(cctx)
		if err != nil {
			return xerrors.Errorf("getting api: %w", err)
		}
		defer acloser()

		ctx := lcli.ReqContext(cctx)

		maddr, err := address.NewFromString(cctx.String("actor"))
		if err != nil {
			return xerrors.Errorf("getting actor address: %w", err)
		}

		minfo, err := api.StateMinerInfo(ctx, maddr, types.EmptyTSK)
		if err != nil {
			return xerrors.Errorf("getting mienr info: %w", err)
		}

		deadline, err := api.StateMinerProvingDeadline(ctx, maddr, types.EmptyTSK)
		if err != nil {
			return xerrors.Errorf("getting deadline: %w", err)
		}

		partitionIndices := cctx.Int64Slice("partitions")
		if len(partitionIndices) <= 0 {
			return fmt.Errorf("must include at least one partition to compact")
		}

		chainHead, err := api.ChainHead(ctx)
		if err != nil {
			return xerrors.Errorf("getting chain head: %w", err)
		}

		checkRand, err := api.StateGetRandomnessFromTickets(ctx, crypto.DomainSeparationTag_PoStChainCommit, deadline.Challenge, nil, chainHead.Key())
		if err != nil {
			return xerrors.Errorf("getting randomness: %w", err)
		}

		proofSize, err := minfo.WindowPoStProofType.ProofSize()
		if err != nil {
			return xerrors.Errorf("getting proof size: %w", err)
		}

		var partitions []miner8.PoStPartition

		emptyProof := []proof.PoStProof{{
			PoStProof:  minfo.WindowPoStProofType,
			ProofBytes: make([]byte, proofSize)}}

		for _, partition := range partitionIndices {
			newPartition := miner8.PoStPartition{
				Index:   uint64(partition),
				Skipped: bitfield.New(),
			}
			partitions = append(partitions, newPartition)
		}

		params := miner8.SubmitWindowedPoStParams{
			Deadline:         deadline.Index,
			Partitions:       partitions,
			Proofs:           emptyProof,
			ChainCommitEpoch: deadline.Challenge,
			ChainCommitRand:  checkRand,
		}

		sp, err := actors.SerializeParams(&params)
		if err != nil {
			return xerrors.Errorf("serializing params: %w", err)
		}

		fmt.Printf("submitting bad PoST for %d paritions\n", len(partitionIndices))
		smsg, err := api.MpoolPushMessage(ctx, &types.Message{
			From:   minfo.Worker,
			To:     maddr,
			Method: builtin.MethodsMiner.SubmitWindowedPoSt,
			Value:  big.Zero(),
			Params: sp,
		}, nil)
		if err != nil {
			return xerrors.Errorf("mpool push: %w", err)
		}

		fmt.Printf("Invalid PoST in message %s\n", smsg.Cid())

		wait, err := api.StateWaitMsg(ctx, smsg.Cid(), 0)
		if err != nil {
			return err
		}

		// check it executed successfully
		if wait.Receipt.ExitCode.IsError() {
			fmt.Println(cctx.App.Writer, "Invalid PoST message failed!")
			return err
		}

		return nil
	},
}

var generateAndSendConsensusFaultCmd = &cli.Command{
	Name:        "generate-and-send-consensus-fault",
	Usage:       "Provided a block CID mined by the miner, will create another block at the same height, and send both block headers to generate a consensus fault.",
	Description: `Note: This is meant for testing purposes and should NOT be used on mainnet or you will be slashed`,
	ArgsUsage:   "blockCID",
	Action: func(cctx *cli.Context) error {
		if cctx.NArg() != 1 {
			return lcli.IncorrectNumArgs(cctx)
		}

		blockCid, err := cid.Parse(cctx.Args().First())
		if err != nil {
			return xerrors.Errorf("getting first arg: %w", err)
		}

		api, acloser, err := lcli.GetFullNodeAPI(cctx)
		if err != nil {
			return xerrors.Errorf("getting chain head: %w", err)
		}
		defer acloser()

		ctx := lcli.ReqContext(cctx)

		blockHeader, err := api.ChainGetBlock(ctx, blockCid)
		if err != nil {
			return xerrors.Errorf("getting block header: %w", err)
		}

		maddr := blockHeader.Miner

		minfo, err := api.StateMinerInfo(ctx, maddr, types.EmptyTSK)
		if err != nil {
			return xerrors.Errorf("getting miner info: %w", err)
		}

		// We are changing one field in the block header, then resigning the new block.
		// This gives two different blocks signed by the same miner at the same height which will result in a consensus fault.
		blockHeaderCopy := *blockHeader
		blockHeaderCopy.ForkSignaling = blockHeader.ForkSignaling + 1

		signingBytes, err := blockHeaderCopy.SigningBytes()
		if err != nil {
			return xerrors.Errorf("getting bytes to sign second block: %w", err)
		}

		sig, err := api.WalletSign(ctx, minfo.Worker, signingBytes)
		if err != nil {
			return xerrors.Errorf("signing second block: %w", err)
		}
		blockHeaderCopy.BlockSig = sig

		buf1 := new(bytes.Buffer)
		err = blockHeader.MarshalCBOR(buf1)
		if err != nil {
			return xerrors.Errorf("marshalling block header 1: %w", err)
		}
		buf2 := new(bytes.Buffer)
		err = blockHeaderCopy.MarshalCBOR(buf2)
		if err != nil {
			return xerrors.Errorf("marshalling block header 2: %w", err)
		}

		params := miner8.ReportConsensusFaultParams{
			BlockHeader1: buf1.Bytes(),
			BlockHeader2: buf2.Bytes(),
		}

		sp, err := actors.SerializeParams(&params)
		if err != nil {
			return xerrors.Errorf("serializing params: %w", err)
		}

		smsg, err := api.MpoolPushMessage(ctx, &types.Message{
			From:   minfo.Worker,
			To:     maddr,
			Method: builtin.MethodsMiner.ReportConsensusFault,
			Value:  big.Zero(),
			Params: sp,
		}, nil)
		if err != nil {
			return xerrors.Errorf("mpool push: %w", err)
		}

		fmt.Printf("Consensus fault reported in message %s\n", smsg.Cid())

		wait, err := api.StateWaitMsg(ctx, smsg.Cid(), 0)
		if err != nil {
			return err
		}

		// check it executed successfully
		if wait.Receipt.ExitCode.IsError() {
			fmt.Println(cctx.App.Writer, "Report consensus fault failed!")
			return err
		}

		return nil
	},
}

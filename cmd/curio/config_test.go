package main

import (
	"testing"

	"github.com/BurntSushi/toml"
	"github.com/filecoin-project/lotus/cmd/curio/deps"
	"github.com/filecoin-project/lotus/node/config"
	"github.com/samber/lo"
	"github.com/stretchr/testify/require"
)

var baseText string = `
[Subsystems]
  # EnableWindowPost enables window post to be executed on this lotus-provider instance. Each machine in the cluster
  # with WindowPoSt enabled will also participate in the window post scheduler. It is possible to have multiple
  # machines with WindowPoSt enabled which will provide redundancy, and in case of multiple partitions per deadline,
  # will allow for parallel processing of partitions.
  # 
  # It is possible to have instances handling both WindowPoSt and WinningPoSt, which can provide redundancy without
  # the need for additional machines. In setups like this it is generally recommended to run
  # partitionsPerDeadline+1 machines.
  #
  # type: bool
  #EnableWindowPost = false

  # type: int
  #WindowPostMaxTasks = 0

  # EnableWinningPost enables winning post to be executed on this lotus-provider instance.
  # Each machine in the cluster with WinningPoSt enabled will also participate in the winning post scheduler.
  # It is possible to mix machines with WindowPoSt and WinningPoSt enabled, for details see the EnableWindowPost
  # documentation.
  #
  # type: bool
  #EnableWinningPost = false

  # type: int
  #WinningPostMaxTasks = 0

  # EnableParkPiece enables the "piece parking" task to run on this node. This task is responsible for fetching
  # pieces from the network and storing them in the storage subsystem until sectors are sealed. This task is
  # only applicable when integrating with boost, and should be enabled on nodes which will hold deal data
  # from boost until sectors containing the related pieces have the TreeD/TreeR constructed.
  # Note that future Curio implementations will have a separate task type for fetching pieces from the internet.
  #
  # type: bool
  #EnableParkPiece = false

  # type: int
  #ParkPieceMaxTasks = 0

  # EnableSealSDR enables SDR tasks to run. SDR is the long sequential computation
  # creating 11 layer files in sector cache directory.
  # 
  # SDR is the first task in the sealing pipeline. It's inputs are just the hash of the
  # unsealed data (CommD), sector number, miner id, and the seal proof type.
  # It's outputs are the 11 layer files in the sector cache directory.
  # 
  # In lotus-miner this was run as part of PreCommit1.
  #
  # type: bool
  #EnableSealSDR = false

  # The maximum amount of SDR tasks that can run simultaneously. Note that the maximum number of tasks will
  # also be bounded by resources available on the machine.
  #
  # type: int
  #SealSDRMaxTasks = 0

  # EnableSealSDRTrees enables the SDR pipeline tree-building task to run.
  # This task handles encoding of unsealed data into last sdr layer and building
  # of TreeR, TreeC and TreeD.
  # 
  # This task runs after SDR
  # TreeD is first computed with optional input of unsealed data
  # TreeR is computed from replica, which is first computed as field
  # addition of the last SDR layer and the bottom layer of TreeD (which is the unsealed data)
  # TreeC is computed from the 11 SDR layers
  # The 3 trees will later be used to compute the PoRep proof.
  # 
  # In case of SyntheticPoRep challenges for PoRep will be pre-generated at this step, and trees and layers
  # will be dropped. SyntheticPoRep works by pre-generating a very large set of challenges (~30GiB on disk)
  # then using a small subset of them for the actual PoRep computation. This allows for significant scratch space
  # saving between PreCommit and PoRep generation at the expense of more computation (generating challenges in this step)
  # 
  # In lotus-miner this was run as part of PreCommit2 (TreeD was run in PreCommit1).
  # Note that nodes with SDRTrees enabled will also answer to Finalize tasks,
  # which just remove unneeded tree data after PoRep is computed.
  #
  # type: bool
  #EnableSealSDRTrees = false

  # The maximum amount of SealSDRTrees tasks that can run simultaneously. Note that the maximum number of tasks will
  # also be bounded by resources available on the machine.
  #
  # type: int
  #SealSDRTreesMaxTasks = 0

  # FinalizeMaxTasks is the maximum amount of finalize tasks that can run simultaneously.
  # The finalize task is enabled on all machines which also handle SDRTrees tasks. Finalize ALWAYS runs on whichever
  # machine holds sector cache files, as it removes unneeded tree data after PoRep is computed.
  # Finalize will run in parallel with the SubmitCommitMsg task.
  #
  # type: int
  #FinalizeMaxTasks = 0

  # EnableSendPrecommitMsg enables the sending of precommit messages to the chain
  # from this lotus-provider instance.
  # This runs after SDRTrees and uses the output CommD / CommR (roots of TreeD / TreeR) for the message
  #
  # type: bool
  #EnableSendPrecommitMsg = false

  # EnablePoRepProof enables the computation of the porep proof
  # 
  # This task runs after interactive-porep seed becomes available, which happens 150 epochs (75min) after the
  # precommit message lands on chain. This task should run on a machine with a GPU. Vanilla PoRep proofs are
  # requested from the machine which holds sector cache files which most likely is the machine which ran the SDRTrees
  # task.
  # 
  # In lotus-miner this was Commit1 / Commit2
  #
  # type: bool
  #EnablePoRepProof = false

  # The maximum amount of PoRepProof tasks that can run simultaneously. Note that the maximum number of tasks will
  # also be bounded by resources available on the machine.
  #
  # type: int
  #PoRepProofMaxTasks = 0

  # EnableSendCommitMsg enables the sending of commit messages to the chain
  # from this lotus-provider instance.
  #
  # type: bool
  #EnableSendCommitMsg = false

  # EnableMoveStorage enables the move-into-long-term-storage task to run on this lotus-provider instance.
  # This tasks should only be enabled on nodes with long-term storage.
  # 
  # The MoveStorage task is the last task in the sealing pipeline. It moves the sealed sector data from the
  # SDRTrees machine into long-term storage. This task runs after the Finalize task.
  #
  # type: bool
  #EnableMoveStorage = false

  # The maximum amount of MoveStorage tasks that can run simultaneously. Note that the maximum number of tasks will
  # also be bounded by resources available on the machine. It is recommended that this value is set to a number which
  # uses all available network (or disk) bandwidth on the machine without causing bottlenecks.
  #
  # type: int
  #MoveStorageMaxTasks = 0

  # EnableWebGui enables the web GUI on this lotus-provider instance. The UI has minimal local overhead, but it should
  # only need to be run on a single machine in the cluster.
  #
  # type: bool
  #EnableWebGui = false

  # The address that should listen for Web GUI requests.
  #
  # type: string
  #GuiAddress = ":4701"


[Fees]
  # type: types.FIL
  #DefaultMaxFee = "0.07 FIL"

  # type: types.FIL
  #MaxPreCommitGasFee = "0.025 FIL"

  # type: types.FIL
  #MaxCommitGasFee = "0.05 FIL"

  # type: types.FIL
  #MaxTerminateGasFee = "0.5 FIL"

  # WindowPoSt is a high-value operation, so the default fee should be high.
  #
  # type: types.FIL
  #MaxWindowPoStGasFee = "5 FIL"

  # type: types.FIL
  #MaxPublishDealsFee = "0.05 FIL"

  [Fees.MaxPreCommitBatchGasFee]
    # type: types.FIL
    #Base = "0 FIL"

    # type: types.FIL
    #PerSector = "0.02 FIL"

  [Fees.MaxCommitBatchGasFee]
    # type: types.FIL
    #Base = "0 FIL"

    # type: types.FIL
    #PerSector = "0.03 FIL"

[[Addresses]]
  #PreCommitControl = []

  #CommitControl = []

  #TerminateControl = []

  #DisableOwnerFallback = false

  #DisableWorkerFallback = false

  MinerAddresses = ["t01013"]


[[Addresses]]
  #PreCommitControl = []

  #CommitControl = []

  #TerminateControl = []

  #DisableOwnerFallback = false

  #DisableWorkerFallback = false

  #MinerAddresses = []


[[Addresses]]
  #PreCommitControl = []

  #CommitControl = []

  #TerminateControl = []

  #DisableOwnerFallback = false

  #DisableWorkerFallback = false

  MinerAddresses = ["t01006"]


[Proving]
  # Maximum number of sector checks to run in parallel. (0 = unlimited)
  # 
  # WARNING: Setting this value too high may make the node crash by running out of stack
  # WARNING: Setting this value too low may make sector challenge reading much slower, resulting in failed PoSt due
  # to late submission.
  # 
  # After changing this option, confirm that the new value works in your setup by invoking
  # 'lotus-miner proving compute window-post 0'
  #
  # type: int
  #ParallelCheckLimit = 32

  # Maximum amount of time a proving pre-check can take for a sector. If the check times out the sector will be skipped
  # 
  # WARNING: Setting this value too low risks in sectors being skipped even though they are accessible, just reading the
  # test challenge took longer than this timeout
  # WARNING: Setting this value too high risks missing PoSt deadline in case IO operations related to this sector are
  # blocked (e.g. in case of disconnected NFS mount)
  #
  # type: Duration
  #SingleCheckTimeout = "10m0s"

  # Maximum amount of time a proving pre-check can take for an entire partition. If the check times out, sectors in
  # the partition which didn't get checked on time will be skipped
  # 
  # WARNING: Setting this value too low risks in sectors being skipped even though they are accessible, just reading the
  # test challenge took longer than this timeout
  # WARNING: Setting this value too high risks missing PoSt deadline in case IO operations related to this partition are
  # blocked or slow
  #
  # type: Duration
  #PartitionCheckTimeout = "20m0s"

  # Disable Window PoSt computation on the lotus-miner process even if no window PoSt workers are present.
  # 
  # WARNING: If no windowPoSt workers are connected, window PoSt WILL FAIL resulting in faulty sectors which will need
  # to be recovered. Before enabling this option, make sure your PoSt workers work correctly.
  # 
  # After changing this option, confirm that the new value works in your setup by invoking
  # 'lotus-miner proving compute window-post 0'
  #
  # type: bool
  #DisableBuiltinWindowPoSt = false

  # Disable Winning PoSt computation on the lotus-miner process even if no winning PoSt workers are present.
  # 
  # WARNING: If no WinningPoSt workers are connected, Winning PoSt WILL FAIL resulting in lost block rewards.
  # Before enabling this option, make sure your PoSt workers work correctly.
  #
  # type: bool
  #DisableBuiltinWinningPoSt = false

  # Disable WindowPoSt provable sector readability checks.
  # 
  # In normal operation, when preparing to compute WindowPoSt, lotus-miner will perform a round of reading challenges
  # from all sectors to confirm that those sectors can be proven. Challenges read in this process are discarded, as
  # we're only interested in checking that sector data can be read.
  # 
  # When using builtin proof computation (no PoSt workers, and DisableBuiltinWindowPoSt is set to false), this process
  # can save a lot of time and compute resources in the case that some sectors are not readable - this is caused by
  # the builtin logic not skipping snark computation when some sectors need to be skipped.
  # 
  # When using PoSt workers, this process is mostly redundant, with PoSt workers challenges will be read once, and
  # if challenges for some sectors aren't readable, those sectors will just get skipped.
  # 
  # Disabling sector pre-checks will slightly reduce IO load when proving sectors, possibly resulting in shorter
  # time to produce window PoSt. In setups with good IO capabilities the effect of this option on proving time should
  # be negligible.
  # 
  # NOTE: It likely is a bad idea to disable sector pre-checks in setups with no PoSt workers.
  # 
  # NOTE: Even when this option is enabled, recovering sectors will be checked before recovery declaration message is
  # sent to the chain
  # 
  # After changing this option, confirm that the new value works in your setup by invoking
  # 'lotus-miner proving compute window-post 0'
  #
  # type: bool
  #DisableWDPoStPreChecks = false

  # Maximum number of partitions to prove in a single SubmitWindowPoSt messace. 0 = network limit (3 in nv21)
  # 
  # A single partition may contain up to 2349 32GiB sectors, or 2300 64GiB sectors.
  # //
  # Note that setting this value lower may result in less efficient gas use - more messages will be sent,
  # to prove each deadline, resulting in more total gas use (but each message will have lower gas limit)
  # 
  # Setting this value above the network limit has no effect
  #
  # type: int
  #MaxPartitionsPerPoStMessage = 0

  # In some cases when submitting DeclareFaultsRecovered messages,
  # there may be too many recoveries to fit in a BlockGasLimit.
  # In those cases it may be necessary to set this value to something low (eg 1);
  # Note that setting this value lower may result in less efficient gas use - more messages will be sent than needed,
  # resulting in more total gas use (but each message will have lower gas limit)
  #
  # type: int
  #MaxPartitionsPerRecoveryMessage = 0

  # Enable single partition per PoSt Message for partitions containing recovery sectors
  # 
  # In cases when submitting PoSt messages which contain recovering sectors, the default network limit may still be
  # too high to fit in the block gas limit. In those cases, it becomes useful to only house the single partition
  # with recovering sectors in the post message
  # 
  # Note that setting this value lower may result in less efficient gas use - more messages will be sent,
  # to prove each deadline, resulting in more total gas use (but each message will have lower gas limit)
  #
  # type: bool
  #SingleRecoveringPartitionPerPostMessage = false


[Journal]
  # Events of the form: "system1:event1,system1:event2[,...]"
  #
  # type: string
  #DisabledEvents = ""


[Apis]
  # ChainApiInfo is the API endpoint for the Lotus daemon.
  #
  # type: []string
  ChainApiInfo = ["eyJhbGciOiJIUzI1NiIsInR5cCI6IkpXVCJ9.eyJBbGxvdyI6WyJyZWFkIiwid3JpdGUiLCJzaWduIiwiYWRtaW4iXX0.T_jmG4DTs9Zjd7rr78862lT7D2U63uz-zqcUKHwcqaU:/dns/localhost/tcp/1234/http"]

  # RPC Secret for the storage subsystem.
  # If integrating with lotus-miner this must match the value from
  # cat ~/.lotusminer/keystore/MF2XI2BNNJ3XILLQOJUXMYLUMU | jq -r .PrivateKey
  #
  # type: string
  StorageRPCSecret = "HxHe8YLHiY0LjHVw/WT/4XQkPGgRyCEYk+xiFi0Ob0o="

`

var bt string = `
[Subsystems]
  # EnableWindowPost enables window post to be executed on this lotus-provider instance. Each machine in the cluster
  # with WindowPoSt enabled will also participate in the window post scheduler. It is possible to have multiple
  # machines with WindowPoSt enabled which will provide redundancy, and in case of multiple partitions per deadline,
  # will allow for parallel processing of partitions.
  # 
  # It is possible to have instances handling both WindowPoSt and WinningPoSt, which can provide redundancy without
  # the need for additional machines. In setups like this it is generally recommended to run
  # partitionsPerDeadline+1 machines.
  #
  # type: bool
  #EnableWindowPost = false

  # type: int
  #WindowPostMaxTasks = 0

  # EnableWinningPost enables winning post to be executed on this lotus-provider instance.
  # Each machine in the cluster with WinningPoSt enabled will also participate in the winning post scheduler.
  # It is possible to mix machines with WindowPoSt and WinningPoSt enabled, for details see the EnableWindowPost
  # documentation.
  #
  # type: bool
  #EnableWinningPost = false

  # type: int
  #WinningPostMaxTasks = 0

  # EnableParkPiece enables the "piece parking" task to run on this node. This task is responsible for fetching
  # pieces from the network and storing them in the storage subsystem until sectors are sealed. This task is
  # only applicable when integrating with boost, and should be enabled on nodes which will hold deal data
  # from boost until sectors containing the related pieces have the TreeD/TreeR constructed.
  # Note that future Curio implementations will have a separate task type for fetching pieces from the internet.
  #
  # type: bool
  #EnableParkPiece = false

  # type: int
  #ParkPieceMaxTasks = 0

  # EnableSealSDR enables SDR tasks to run. SDR is the long sequential computation
  # creating 11 layer files in sector cache directory.
  # 
  # SDR is the first task in the sealing pipeline. It's inputs are just the hash of the
  # unsealed data (CommD), sector number, miner id, and the seal proof type.
  # It's outputs are the 11 layer files in the sector cache directory.
  # 
  # In lotus-miner this was run as part of PreCommit1.
  #
  # type: bool
  #EnableSealSDR = false

  # The maximum amount of SDR tasks that can run simultaneously. Note that the maximum number of tasks will
  # also be bounded by resources available on the machine.
  #
  # type: int
  #SealSDRMaxTasks = 0

  # EnableSealSDRTrees enables the SDR pipeline tree-building task to run.
  # This task handles encoding of unsealed data into last sdr layer and building
  # of TreeR, TreeC and TreeD.
  # 
  # This task runs after SDR
  # TreeD is first computed with optional input of unsealed data
  # TreeR is computed from replica, which is first computed as field
  # addition of the last SDR layer and the bottom layer of TreeD (which is the unsealed data)
  # TreeC is computed from the 11 SDR layers
  # The 3 trees will later be used to compute the PoRep proof.
  # 
  # In case of SyntheticPoRep challenges for PoRep will be pre-generated at this step, and trees and layers
  # will be dropped. SyntheticPoRep works by pre-generating a very large set of challenges (~30GiB on disk)
  # then using a small subset of them for the actual PoRep computation. This allows for significant scratch space
  # saving between PreCommit and PoRep generation at the expense of more computation (generating challenges in this step)
  # 
  # In lotus-miner this was run as part of PreCommit2 (TreeD was run in PreCommit1).
  # Note that nodes with SDRTrees enabled will also answer to Finalize tasks,
  # which just remove unneeded tree data after PoRep is computed.
  #
  # type: bool
  #EnableSealSDRTrees = false

  # The maximum amount of SealSDRTrees tasks that can run simultaneously. Note that the maximum number of tasks will
  # also be bounded by resources available on the machine.
  #
  # type: int
  #SealSDRTreesMaxTasks = 0

  # FinalizeMaxTasks is the maximum amount of finalize tasks that can run simultaneously.
  # The finalize task is enabled on all machines which also handle SDRTrees tasks. Finalize ALWAYS runs on whichever
  # machine holds sector cache files, as it removes unneeded tree data after PoRep is computed.
  # Finalize will run in parallel with the SubmitCommitMsg task.
  #
  # type: int
  #FinalizeMaxTasks = 0

  # EnableSendPrecommitMsg enables the sending of precommit messages to the chain
  # from this lotus-provider instance.
  # This runs after SDRTrees and uses the output CommD / CommR (roots of TreeD / TreeR) for the message
  #
  # type: bool
  #EnableSendPrecommitMsg = false

  # EnablePoRepProof enables the computation of the porep proof
  # 
  # This task runs after interactive-porep seed becomes available, which happens 150 epochs (75min) after the
  # precommit message lands on chain. This task should run on a machine with a GPU. Vanilla PoRep proofs are
  # requested from the machine which holds sector cache files which most likely is the machine which ran the SDRTrees
  # task.
  # 
  # In lotus-miner this was Commit1 / Commit2
  #
  # type: bool
  #EnablePoRepProof = false

  # The maximum amount of PoRepProof tasks that can run simultaneously. Note that the maximum number of tasks will
  # also be bounded by resources available on the machine.
  #
  # type: int
  #PoRepProofMaxTasks = 0

  # EnableSendCommitMsg enables the sending of commit messages to the chain
  # from this lotus-provider instance.
  #
  # type: bool
  #EnableSendCommitMsg = false

  # EnableMoveStorage enables the move-into-long-term-storage task to run on this lotus-provider instance.
  # This tasks should only be enabled on nodes with long-term storage.
  # 
  # The MoveStorage task is the last task in the sealing pipeline. It moves the sealed sector data from the
  # SDRTrees machine into long-term storage. This task runs after the Finalize task.
  #
  # type: bool
  #EnableMoveStorage = false

  # The maximum amount of MoveStorage tasks that can run simultaneously. Note that the maximum number of tasks will
  # also be bounded by resources available on the machine. It is recommended that this value is set to a number which
  # uses all available network (or disk) bandwidth on the machine without causing bottlenecks.
  #
  # type: int
  #MoveStorageMaxTasks = 0

  # EnableWebGui enables the web GUI on this lotus-provider instance. The UI has minimal local overhead, but it should
  # only need to be run on a single machine in the cluster.
  #
  # type: bool
  #EnableWebGui = false

  # The address that should listen for Web GUI requests.
  #
  # type: string
  #GuiAddress = ":4701"


[Fees]
  # type: types.FIL
  #DefaultMaxFee = "0.07 FIL"

  # type: types.FIL
  #MaxPreCommitGasFee = "0.025 FIL"

  # type: types.FIL
  #MaxCommitGasFee = "0.05 FIL"

  # type: types.FIL
  #MaxTerminateGasFee = "0.5 FIL"

  # WindowPoSt is a high-value operation, so the default fee should be high.
  #
  # type: types.FIL
  #MaxWindowPoStGasFee = "5 FIL"

  # type: types.FIL
  #MaxPublishDealsFee = "0.05 FIL"

  [Fees.MaxPreCommitBatchGasFee]
    # type: types.FIL
    #Base = "0 FIL"

    # type: types.FIL
    #PerSector = "0.02 FIL"

  [Fees.MaxCommitBatchGasFee]
    # type: types.FIL
    #Base = "0 FIL"

    # type: types.FIL
    #PerSector = "0.03 FIL"


[[Addresses]]
  #PreCommitControl = []

  #CommitControl = []

  #TerminateControl = []

  #DisableOwnerFallback = false

  #DisableWorkerFallback = false

  MinerAddresses = ["t01013"]


[Proving]
  # Maximum number of sector checks to run in parallel. (0 = unlimited)
  # 
  # WARNING: Setting this value too high may make the node crash by running out of stack
  # WARNING: Setting this value too low may make sector challenge reading much slower, resulting in failed PoSt due
  # to late submission.
  # 
  # After changing this option, confirm that the new value works in your setup by invoking
  # 'lotus-miner proving compute window-post 0'
  #
  # type: int
  #ParallelCheckLimit = 32

  # Maximum amount of time a proving pre-check can take for a sector. If the check times out the sector will be skipped
  # 
  # WARNING: Setting this value too low risks in sectors being skipped even though they are accessible, just reading the
  # test challenge took longer than this timeout
  # WARNING: Setting this value too high risks missing PoSt deadline in case IO operations related to this sector are
  # blocked (e.g. in case of disconnected NFS mount)
  #
  # type: Duration
  #SingleCheckTimeout = "10m0s"

  # Maximum amount of time a proving pre-check can take for an entire partition. If the check times out, sectors in
  # the partition which didn't get checked on time will be skipped
  # 
  # WARNING: Setting this value too low risks in sectors being skipped even though they are accessible, just reading the
  # test challenge took longer than this timeout
  # WARNING: Setting this value too high risks missing PoSt deadline in case IO operations related to this partition are
  # blocked or slow
  #
  # type: Duration
  #PartitionCheckTimeout = "20m0s"

  # Disable Window PoSt computation on the lotus-miner process even if no window PoSt workers are present.
  # 
  # WARNING: If no windowPoSt workers are connected, window PoSt WILL FAIL resulting in faulty sectors which will need
  # to be recovered. Before enabling this option, make sure your PoSt workers work correctly.
  # 
  # After changing this option, confirm that the new value works in your setup by invoking
  # 'lotus-miner proving compute window-post 0'
  #
  # type: bool
  #DisableBuiltinWindowPoSt = false

  # Disable Winning PoSt computation on the lotus-miner process even if no winning PoSt workers are present.
  # 
  # WARNING: If no WinningPoSt workers are connected, Winning PoSt WILL FAIL resulting in lost block rewards.
  # Before enabling this option, make sure your PoSt workers work correctly.
  #
  # type: bool
  #DisableBuiltinWinningPoSt = false

  # Disable WindowPoSt provable sector readability checks.
  # 
  # In normal operation, when preparing to compute WindowPoSt, lotus-miner will perform a round of reading challenges
  # from all sectors to confirm that those sectors can be proven. Challenges read in this process are discarded, as
  # we're only interested in checking that sector data can be read.
  # 
  # When using builtin proof computation (no PoSt workers, and DisableBuiltinWindowPoSt is set to false), this process
  # can save a lot of time and compute resources in the case that some sectors are not readable - this is caused by
  # the builtin logic not skipping snark computation when some sectors need to be skipped.
  # 
  # When using PoSt workers, this process is mostly redundant, with PoSt workers challenges will be read once, and
  # if challenges for some sectors aren't readable, those sectors will just get skipped.
  # 
  # Disabling sector pre-checks will slightly reduce IO load when proving sectors, possibly resulting in shorter
  # time to produce window PoSt. In setups with good IO capabilities the effect of this option on proving time should
  # be negligible.
  # 
  # NOTE: It likely is a bad idea to disable sector pre-checks in setups with no PoSt workers.
  # 
  # NOTE: Even when this option is enabled, recovering sectors will be checked before recovery declaration message is
  # sent to the chain
  # 
  # After changing this option, confirm that the new value works in your setup by invoking
  # 'lotus-miner proving compute window-post 0'
  #
  # type: bool
  #DisableWDPoStPreChecks = false

  # Maximum number of partitions to prove in a single SubmitWindowPoSt messace. 0 = network limit (3 in nv21)
  # 
  # A single partition may contain up to 2349 32GiB sectors, or 2300 64GiB sectors.
  # //
  # Note that setting this value lower may result in less efficient gas use - more messages will be sent,
  # to prove each deadline, resulting in more total gas use (but each message will have lower gas limit)
  # 
  # Setting this value above the network limit has no effect
  #
  # type: int
  #MaxPartitionsPerPoStMessage = 0

  # In some cases when submitting DeclareFaultsRecovered messages,
  # there may be too many recoveries to fit in a BlockGasLimit.
  # In those cases it may be necessary to set this value to something low (eg 1);
  # Note that setting this value lower may result in less efficient gas use - more messages will be sent than needed,
  # resulting in more total gas use (but each message will have lower gas limit)
  #
  # type: int
  #MaxPartitionsPerRecoveryMessage = 0

  # Enable single partition per PoSt Message for partitions containing recovery sectors
  # 
  # In cases when submitting PoSt messages which contain recovering sectors, the default network limit may still be
  # too high to fit in the block gas limit. In those cases, it becomes useful to only house the single partition
  # with recovering sectors in the post message
  # 
  # Note that setting this value lower may result in less efficient gas use - more messages will be sent,
  # to prove each deadline, resulting in more total gas use (but each message will have lower gas limit)
  #
  # type: bool
  #SingleRecoveringPartitionPerPostMessage = false


[Journal]
  # Events of the form: "system1:event1,system1:event2[,...]"
  #
  # type: string
  #DisabledEvents = ""


[Apis]
  # ChainApiInfo is the API endpoint for the Lotus daemon.
  #
  # type: []string
  ChainApiInfo = ["eyJhbGciOiJIUzI1NiIsInR5cCI6IkpXVCJ9.eyJBbGxvdyI6WyJyZWFkIiwid3JpdGUiLCJzaWduIiwiYWRtaW4iXX0.T_jmG4DTs9Zjd7rr78862lT7D2U63uz-zqcUKHwcqaU:/dns/localhost/tcp/1234/http"]

  # RPC Secret for the storage subsystem.
  # If integrating with lotus-miner this must match the value from
  # cat ~/.lotusminer/keystore/MF2XI2BNNJ3XILLQOJUXMYLUMU | jq -r .PrivateKey
  #
  # type: string
  StorageRPCSecret = "HxHe8YLHiY0LjHVw/WT/4XQkPGgRyCEYk+xiFi0Ob0o="

`

func TestConfig(t *testing.T) {
	curioConfig := config.DefaultCurioConfig()

	baseCfg := config.DefaultCurioConfig()

	curioConfig.Addresses = append(curioConfig.Addresses, config.CurioAddresses{
		PreCommitControl:      []string{},
		CommitControl:         []string{},
		TerminateControl:      []string{"t3qroiebizgkz7pvj26reg5r5mqiftrt5hjdske2jzjmlacqr2qj7ytjncreih2mvujxoypwpfusmwpipvxncq"},
		DisableOwnerFallback:  false,
		DisableWorkerFallback: false,
		MinerAddresses:        []string{"t01000"},
	})

	_, err := deps.LoadConfigWithUpgrades(baseText, baseCfg)
	//_, err := toml.Decode(baseText, baseCfg)
	require.NoError(t, err)

	baseCfg.Addresses = append(baseCfg.Addresses, curioConfig.Addresses...)
	baseCfg.Addresses = lo.Filter(baseCfg.Addresses, func(a config.CurioAddresses, _ int) bool {
		return len(a.MinerAddresses) > 0
	})

	_, err = config.ConfigUpdate(baseCfg, config.DefaultCurioConfig(), config.Commented(true), config.DefaultKeepUncommented(), config.NoEnv())
	require.NoError(t, err)
	//t.Log(string(cb))

	base := config.DefaultCurioConfig()

	_, err = toml.Decode(bt, base)
	require.NoError(t, err)

	base.Addresses = append(base.Addresses, curioConfig.Addresses...)
	base.Addresses = lo.Filter(base.Addresses, func(a config.CurioAddresses, _ int) bool {
		return len(a.MinerAddresses) > 0
	})

	_, err = config.ConfigUpdate(base, config.DefaultCurioConfig(), config.Commented(true), config.DefaultKeepUncommented(), config.NoEnv())
	require.NoError(t, err)
}

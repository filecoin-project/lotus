package alertmanager

import (
	"database/sql"
	"fmt"
	"strings"

	"github.com/BurntSushi/toml"
	"github.com/dustin/go-humanize"
	"golang.org/x/xerrors"

	"github.com/filecoin-project/go-address"
	"github.com/filecoin-project/go-state-types/abi"
	"github.com/filecoin-project/go-state-types/big"

	"github.com/filecoin-project/lotus/node/config"
)

// balanceCheck retrieves the machine details from the database and performs balance checks on unique addresses.
// It populates the alert map with any errors encountered during the process and with any alerts related to low wallet balance and missing wallets.
// The alert map key is "Balance Check".
// It queries the database for the configuration of each layer and decodes it using the toml.Decode function.
// It then iterates over the addresses in the configuration and curates a list of unique addresses.
// If an address is not found in the chain node, it adds an alert to the alert map.
// If the balance of an address is below MinimumWalletBalance, it adds an alert to the alert map.
// If there are any errors encountered during the process, the err field of the alert map is populated.
func balanceCheck(al *alerts) {
	Name := "Balance Check"
	al.alertMap[Name] = &alertOut{}

	var ret string

	uniqueAddrs, _, err := al.getAddresses()
	if err != nil {
		al.alertMap[Name].err = err
		return
	}

	for _, addrStr := range uniqueAddrs {
		addr, err := address.NewFromString(addrStr)
		if err != nil {
			al.alertMap[Name].err = xerrors.Errorf("failed to parse address: %w", err)
			return
		}

		has, err := al.api.WalletHas(al.ctx, addr)
		if err != nil {
			al.alertMap[Name].err = err
			return
		}

		if !has {
			ret += fmt.Sprintf("Wallet %s was not found in chain node. ", addrStr)
		}

		balance, err := al.api.WalletBalance(al.ctx, addr)
		if err != nil {
			al.alertMap[Name].err = err
		}

		if abi.TokenAmount(al.cfg.MinimumWalletBalance).GreaterThanEqual(balance) {
			ret += fmt.Sprintf("Balance for wallet %s is below 5 Fil. ", addrStr)
		}
	}
	if ret != "" {
		al.alertMap[Name].alertString = ret
	}
	return
}

// taskFailureCheck retrieves the task failure counts from the database for a specific time period.
// It then checks for specific sealing tasks and tasks with more than 5 failures to generate alerts.
func taskFailureCheck(al *alerts) {
	Name := "TaskFailures"
	al.alertMap[Name] = &alertOut{}

	type taskFailure struct {
		Machine  string `db:"completed_by_host_and_port"`
		Name     string `db:"name"`
		Failures int    `db:"failed_tasks_count"`
	}

	var taskFailures []taskFailure

	err := al.db.Select(al.ctx, &taskFailures, `
								SELECT completed_by_host_and_port, name, COUNT(*) AS failed_count
								FROM harmony_task_history
								WHERE result = FALSE
								  AND work_end >= NOW() - $1::interval
								GROUP BY completed_by_host_and_port, name
								ORDER BY completed_by_host_and_port, name;`, fmt.Sprintf("%f Minutes", AlertMangerInterval.Minutes()))
	if err != nil {
		al.alertMap[Name].err = xerrors.Errorf("getting failed task count: %w", err)
		return
	}

	mmap := make(map[string]int)
	tmap := make(map[string]int)

	if len(taskFailures) > 0 {
		for _, tf := range taskFailures {
			_, ok := tmap[tf.Name]
			if !ok {
				tmap[tf.Name] = tf.Failures
			} else {
				tmap[tf.Name] += tf.Failures
			}
			_, ok = mmap[tf.Machine]
			if !ok {
				mmap[tf.Machine] = tf.Failures
			} else {
				mmap[tf.Machine] += tf.Failures
			}
		}
	}

	sealingTasks := []string{"SDR", "TreeD", "TreeRC", "PreCommitSubmit", "PoRep", "Finalize", "MoveStorage", "CommitSubmit", "WdPost", "ParkPiece"}
	contains := func(s []string, e string) bool {
		for _, a := range s {
			if a == e {
				return true
			}
		}
		return false
	}

	// Alerts for any sealing pipeline failures. Other tasks should have at least 5 failures for an alert
	for name, count := range tmap {
		if contains(sealingTasks, name) {
			al.alertMap[Name].alertString += fmt.Sprintf("Task: %s, Failures: %d. ", name, count)
		}
		if count > 5 {
			al.alertMap[Name].alertString += fmt.Sprintf("Task: %s, Failures: %d. ", name, count)
		}
	}

	// Alert if a machine failed more than 5 tasks
	for name, count := range tmap {
		if count > 5 {
			al.alertMap[Name].alertString += fmt.Sprintf("Machine: %s, Failures: %d. ", name, count)
		}
	}

	return
}

// permanentStorageCheck retrieves the storage details from the database and checks if there is sufficient space for sealing sectors.
// It queries the database for the available storage for all storage paths that can store data.
// It queries the database for sectors being sealed that have not been finalized yet.
// For each sector, it calculates the required space for sealing based on the sector size.
// It checks if there is enough available storage for each sector and updates the sectorMap accordingly.
// If any sectors are unaccounted for, it calculates the total missing space and adds an alert to the alert map.
func permanentStorageCheck(al *alerts) {
	Name := "PermanentStorageSpace"
	// Get all storage path for permanent storages
	type storage struct {
		ID        string `db:"storage_id"`
		Available int64  `db:"available"`
	}

	var storages []storage

	err := al.db.Select(al.ctx, &storages, `
								SELECT storage_id, available
								FROM storage_path
								WHERE can_store = TRUE;`)
	if err != nil {
		al.alertMap[Name].err = xerrors.Errorf("getting storage details: %w", err)
		return
	}

	type sector struct {
		Miner  abi.ActorID             `db:"sp_id"`
		Number abi.SectorNumber        `db:"sector_number"`
		Proof  abi.RegisteredSealProof `db:"reg_seal_proof"`
	}

	var sectors []sector

	err = al.db.Select(al.ctx, &sectors, `
								SELECT sp_id, sector_number, reg_seal_proof
								FROM sectors_sdr_pipeline
								WHERE after_move_storage = FALSE;`)
	if err != nil {
		al.alertMap[Name].err = xerrors.Errorf("getting sectors being sealed: %w", err)
		return
	}

	type sm struct {
		s    sector
		size int64
	}

	sectorMap := make(map[sm]bool)

	for _, sec := range sectors {
		space := int64(0)
		sec := sec
		sectorSize, err := sec.Proof.SectorSize()
		if err != nil {
			space = int64(64<<30)*2 + int64(200<<20) // Assume 64 GiB sector
		} else {
			space = int64(sectorSize)*2 + int64(200<<20) // sealed + unsealed + cache
		}

		key := sm{s: sec, size: space}

		sectorMap[key] = false

		for _, strg := range storages {
			if space > strg.Available {
				strg.Available -= space
				sectorMap[key] = true
			}
		}
	}

	missingSpace := big.NewInt(0)
	for sec, accounted := range sectorMap {
		if !accounted {
			big.Add(missingSpace, big.NewInt(sec.size))
		}
	}

	if missingSpace.GreaterThan(big.NewInt(0)) {
		al.alertMap[Name].alertString = fmt.Sprintf("Insufficient storage space for sealing sectors. Additional %s required.", humanize.Bytes(missingSpace.Uint64()))
	}
}

// getAddresses retrieves machine details from the database, stores them in an array and compares layers for uniqueness.
// It employs addrMap to handle unique addresses, and generated slices for configuration fields and MinerAddresses.
// The function iterates over layers, storing decoded configuration and verifying address existence in addrMap.
// It ends by returning unique addresses and miner slices.
func (al *alerts) getAddresses() ([]string, []string, error) {
	// MachineDetails represents the structure of data received from the SQL query.
	type machineDetail struct {
		ID          int
		HostAndPort string
		Layers      string
	}
	var machineDetails []machineDetail

	// Get all layers in use
	err := al.db.Select(al.ctx, &machineDetails, `
				SELECT m.id, m.host_and_port, d.layers
				FROM harmony_machines m
				LEFT JOIN harmony_machine_details d ON m.id = d.machine_id;`)
	if err != nil {
		return nil, nil, xerrors.Errorf("getting config layers for all machines: %w", err)
	}

	// UniqueLayers takes an array of MachineDetails and returns a slice of unique layers.

	layerMap := make(map[string]bool)
	var uniqueLayers []string

	// Get unique layers in use
	for _, machine := range machineDetails {
		machine := machine
		// Split the Layers field into individual layers
		layers := strings.Split(machine.Layers, ",")
		for _, layer := range layers {
			layer = strings.TrimSpace(layer)
			if _, exists := layerMap[layer]; !exists && layer != "" {
				layerMap[layer] = true
				uniqueLayers = append(uniqueLayers, layer)
			}
		}
	}

	addrMap := make(map[string]bool)
	var uniqueAddrs []string
	var miners []string

	// Get all unique addresses
	for _, layer := range uniqueLayers {
		text := ""
		cfg := config.DefaultCurioConfig()
		err := al.db.QueryRow(al.ctx, `SELECT config FROM harmony_config WHERE title=$1`, layer).Scan(&text)
		if err != nil {
			if strings.Contains(err.Error(), sql.ErrNoRows.Error()) {
				return nil, nil, xerrors.Errorf("missing layer '%s' ", layer)
			}
			return nil, nil, fmt.Errorf("could not read layer '%s': %w", layer, err)
		}

		_, err = toml.Decode(text, cfg)
		if err != nil {
			return nil, nil, fmt.Errorf("could not read layer, bad toml %s: %w", layer, err)
		}

		for i := range cfg.Addresses {
			prec := cfg.Addresses[i].PreCommitControl
			com := cfg.Addresses[i].CommitControl
			term := cfg.Addresses[i].TerminateControl
			miner := cfg.Addresses[i].MinerAddresses
			if prec != nil {
				for j := range prec {
					if _, ok := addrMap[prec[j]]; !ok && prec[j] != "" {
						addrMap[prec[j]] = true
						uniqueAddrs = append(uniqueAddrs, prec[j])
					}
				}
			}
			if com != nil {
				for j := range com {
					if _, ok := addrMap[com[j]]; !ok && com[j] != "" {
						addrMap[com[j]] = true
						uniqueAddrs = append(uniqueAddrs, com[j])
					}
				}
			}
			if term != nil {
				for j := range term {
					if _, ok := addrMap[term[j]]; !ok && term[j] != "" {
						addrMap[term[j]] = true
						uniqueAddrs = append(uniqueAddrs, term[j])
					}
				}
			}
			if miner != nil {
				for j := range miner {
					if _, ok := addrMap[miner[j]]; !ok && miner[j] != "" {
						addrMap[miner[j]] = true
						miners = append(miners, miner[j])
					}
				}
			}
		}
	}
	return uniqueAddrs, miners, nil
}

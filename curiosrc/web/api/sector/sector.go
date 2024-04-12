package sector

import (
	"encoding/json"
	"net/http"

	"github.com/gorilla/mux"

	"github.com/filecoin-project/go-address"
	"github.com/filecoin-project/go-state-types/abi"

	"github.com/filecoin-project/lotus/cmd/curio/deps"
	"github.com/filecoin-project/lotus/curiosrc/web/api/apihelper"
)

type cfg struct {
	*deps.Deps
}

func Routes(r *mux.Router, deps *deps.Deps) {
	c := &cfg{deps}
	// At menu.html:
	r.Methods("GET").Path("/all").HandlerFunc(c.getSectors)
}

func (c *cfg) getSectors(w http.ResponseWriter, r *http.Request) {
	// TODO get sector info from chain and from database, then fold them together
	// and return the result.
	type sector struct {
		MinerID           int64          `db:"miner_id"`
		SectorNum         int64          `db:"sector_num"`
		SectorFiletype    int            `db:"sector_filetype"` // Useless?
		StorageID         string         `db:"storage_id"`      // map to serverName
		IsPrimary         bool           `db:"is_primary"`      // Useless?
		ExpiresAt         abi.ChainEpoch // map to Duration
		IsOnChain         bool
		SealProof         abi.RegisteredSealProof
		Activation        abi.ChainEpoch // map to time.Time
		DealIDs           []abi.DealID   // Expansion-only option?
		ExpectedDayReward abi.TokenAmount
		MissingFromDisk   bool
		IsFilPlus         bool
	}
	var sectors []sector
	apihelper.OrHTTPFail(w, c.DB.Select(r.Context(), &sectors, `SELECT 
		miner_id, sector_num, sector_filetype, storage_id, is_primary
		FROM sector_location`))
	minerToAddr := map[int64]address.Address{}
	head, err := c.Full.ChainHead(r.Context())
	apihelper.OrHTTPFail(w, err)

	type sectorID struct {
		mID  int64
		sNum uint64
	}
	sectorIdx := map[sectorID]int{}
	for i, s := range sectors {
		sectorIdx[sectorID{s.MinerID, uint64(s.SectorNum)}] = i
		if _, ok := minerToAddr[s.MinerID]; !ok {
			minerToAddr[s.MinerID], err = address.NewIDAddress(uint64(s.MinerID))
			apihelper.OrHTTPFail(w, err)
		}
	}

	for minerID, maddr := range minerToAddr {
		onChainInfo, err := c.Full.StateMinerSectors(r.Context(), maddr, nil, head.Key())
		apihelper.OrHTTPFail(w, err)
		for _, chainy := range onChainInfo {
			if i, ok := sectorIdx[sectorID{minerID, uint64(chainy.SectorNumber)}]; ok {
				sectors[i].IsOnChain = true
				sectors[i].ExpiresAt = chainy.Expiration
				sectors[i].SealProof = chainy.SealProof
				sectors[i].Activation = chainy.Activation
				sectors[i].DealIDs = chainy.DealIDs
				sectors[i].IsFilPlus = chainy.VerifiedDealWeight.GreaterThan(chainy.DealWeight)
				sectors[i].ExpectedDayReward = chainy.ExpectedDayReward
				// too many, not enough?
			} else {
				// sector is on chain but not in db
				sectors = append(sectors, sector{
					MinerID:           minerID,
					SectorNum:         int64(chainy.SectorNumber),
					IsOnChain:         true,
					MissingFromDisk:   true,
					ExpiresAt:         chainy.Expiration,
					SealProof:         chainy.SealProof,
					Activation:        chainy.Activation,
					DealIDs:           chainy.DealIDs,
					ExpectedDayReward: chainy.ExpectedDayReward,
					IsFilPlus:         chainy.VerifiedDealWeight.GreaterThan(chainy.DealWeight),
				})
			}
			/*
				info, err := c.Full.StateSectorGetInfo(r.Context(), minerToAddr[s], abi.SectorNumber(uint64(sectors[i].SectorNum)), headKey)
				if err != nil {
					sectors[i].IsValid = false
					continue
				}*/
		}
	}
	apihelper.OrHTTPFail(w, json.NewEncoder(w).Encode(map[string]any{"data": sectors}))
}

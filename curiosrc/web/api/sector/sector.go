package sector

import (
	"context"
	"encoding/json"
	"net/http"
	"sync"
	"time"

	"github.com/gorilla/mux"
	"github.com/samber/lo"

	"github.com/filecoin-project/go-address"
	"github.com/filecoin-project/go-state-types/abi"

	"github.com/filecoin-project/lotus/chain/actors/builtin/miner"
	"github.com/filecoin-project/lotus/chain/types"
	"github.com/filecoin-project/lotus/cli/spcli"
	"github.com/filecoin-project/lotus/cmd/curio/deps"
	"github.com/filecoin-project/lotus/curiosrc/web/api/apihelper"
	"github.com/filecoin-project/lotus/storage/sealer/storiface"
)

type cfg struct {
	*deps.Deps
}

func Routes(r *mux.Router, deps *deps.Deps) {
	c := &cfg{deps}
	// At menu.html:
	r.Methods("GET").Path("/all").HandlerFunc(c.getSectors)
	r.Methods("POST").Path("/terminate").HandlerFunc(c.terminateSectors)
}

func (c *cfg) terminateSectors(w http.ResponseWriter, r *http.Request) {
	var in []struct {
		MinerID int
		Sector  int
	}
	apihelper.OrHTTPFail(w, json.NewDecoder(r.Body).Decode(&in))
	toDel := map[int][]int{}
	for _, s := range in {
		toDel[s.MinerID] = append(toDel[s.MinerID], s.Sector)
	}

	for minerInt, sectors := range toDel {
		maddr, err := address.NewIDAddress(uint64(minerInt))
		apihelper.OrHTTPFail(w, err)
		mi, err := c.Full.StateMinerInfo(r.Context(), maddr, types.EmptyTSK)
		apihelper.OrHTTPFail(w, err)
		_, err = spcli.TerminateSectors(r.Context(), c.Full, maddr, sectors, mi.Worker)
		apihelper.OrHTTPFail(w, err)
		for _, sectorNumber := range sectors {
			id := abi.SectorID{Miner: abi.ActorID(minerInt), Number: abi.SectorNumber(sectorNumber)}
			apihelper.OrHTTPFail(w, c.Stor.Remove(r.Context(), id, storiface.FTAll, true, nil))
		}
	}
}

func (c *cfg) getSectors(w http.ResponseWriter, r *http.Request) {
	// TODO get sector info from chain and from database, then fold them together
	// and return the result.
	type sector struct {
		MinerID        int64 `db:"miner_id"`
		SectorNum      int64 `db:"sector_num"`
		SectorFiletype int   `db:"sector_filetype" json:"-"` // Useless?
		HasSealed      bool
		HasUnsealed    bool
		HasSnap        bool
		ExpiresAt      abi.ChainEpoch // map to Duration
		IsOnChain      bool
		//StorageID         string         `db:"storage_id"` // map to serverName
		// Activation        abi.ChainEpoch // map to time.Time. advanced view only
		// DealIDs           []abi.DealID //  advanced view only
		ExpectedDayReward abi.TokenAmount
		IsFilPlus         bool
		SealProof         abi.RegisteredSealProof
		SealInfo          string
	}
	var sectors []sector
	apihelper.OrHTTPFail(w, c.DB.Select(r.Context(), &sectors, `SELECT 
		miner_id, sector_num, SUM(sector_filetype) as sector_filetype  
		FROM sector_location 
		GROUP BY miner_id, sector_num 
		ORDER BY miner_id, sector_num`))
	minerToAddr := map[int64]address.Address{}
	head, err := c.Full.ChainHead(r.Context())
	apihelper.OrHTTPFail(w, err)

	type sectorID struct {
		mID  int64
		sNum uint64
	}
	sectorIdx := map[sectorID]int{}
	for i, s := range sectors {
		sectors[i].HasSealed = s.SectorFiletype&int(storiface.FTSealed) != 0 || s.SectorFiletype&int(storiface.FTUpdate) != 0
		sectors[i].HasUnsealed = s.SectorFiletype&int(storiface.FTUnsealed) != 0
		sectors[i].HasSnap = s.SectorFiletype&int(storiface.FTUpdate) != 0
		if ss, err := s.SealProof.SectorSize(); err == nil {
			sectors[i].SealInfo = ss.ShortString()
		}
		sectorIdx[sectorID{s.MinerID, uint64(s.SectorNum)}] = i
		if _, ok := minerToAddr[s.MinerID]; !ok {
			minerToAddr[s.MinerID], err = address.NewIDAddress(uint64(s.MinerID))
			apihelper.OrHTTPFail(w, err)
		}
	}

	for minerID, maddr := range minerToAddr {
		onChainInfo, err := c.getCachedSectorInfo(w, r, maddr, head.Key())
		apihelper.OrHTTPFail(w, err)
		for _, chainy := range onChainInfo {
			if i, ok := sectorIdx[sectorID{minerID, uint64(chainy.SectorNumber)}]; ok {
				sectors[i].IsOnChain = true
				sectors[i].ExpiresAt = chainy.Expiration
				sectors[i].SealProof = chainy.SealProof
				sectors[i].IsFilPlus = chainy.VerifiedDealWeight.GreaterThan(chainy.DealWeight)
				sectors[i].ExpectedDayReward = chainy.ExpectedDayReward
				// too many, not enough?
			} else {
				// sector is on chain but not in db
				sectors = append(sectors, sector{
					MinerID:           minerID,
					SectorNum:         int64(chainy.SectorNumber),
					IsOnChain:         true,
					ExpiresAt:         chainy.Expiration,
					SealProof:         chainy.SealProof,
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

type sectorCacheEntry struct {
	sectors []*miner.SectorOnChainInfo
	loading chan struct{}
	time.Time
}

const cacheTimeout = 30 * time.Minute

var mx sync.Mutex
var sectorInfoCache = map[address.Address]sectorCacheEntry{}

// getCachedSectorInfo returns the sector info for the given miner address,
// either from the cache or by querying the chain.
// Cache can be invalidated by setting the "sector_refresh" cookie to "true".
// This is thread-safe.
// Parallel requests share the chain's first response.
func (c *cfg) getCachedSectorInfo(w http.ResponseWriter, r *http.Request, maddr address.Address, headKey types.TipSetKey) ([]*miner.SectorOnChainInfo, error) {
	mx.Lock()
	v, ok := sectorInfoCache[maddr]
	mx.Unlock()

	if ok && v.loading != nil {
		<-v.loading
		mx.Lock()
		v, ok = sectorInfoCache[maddr]
		mx.Unlock()
	}

	shouldRefreshCookie, found := lo.Find(r.Cookies(), func(item *http.Cookie) bool { return item.Name == "sector_refresh" })
	shouldRefresh := found && shouldRefreshCookie.Value == "true"
	w.Header().Set("Set-Cookie", "sector_refresh=; Max-Age=0; Path=/")

	if !ok || time.Since(v.Time) > cacheTimeout || shouldRefresh {
		v = sectorCacheEntry{nil, make(chan struct{}), time.Now()}
		mx.Lock()
		sectorInfoCache[maddr] = v
		mx.Unlock()

		// Intentionally not using the context from the request, as this is a cache
		onChainInfo, err := c.Full.StateMinerSectors(context.Background(), maddr, nil, headKey)
		if err != nil {
			mx.Lock()
			delete(sectorInfoCache, maddr)
			close(v.loading)
			mx.Unlock()
			return nil, err
		}
		mx.Lock()
		sectorInfoCache[maddr] = sectorCacheEntry{onChainInfo, nil, time.Now()}
		close(v.loading)
		mx.Unlock()
		return onChainInfo, nil
	}
	return v.sectors, nil
}

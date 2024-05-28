package sector

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"sync"
	"time"

	"github.com/docker/go-units"
	"github.com/gorilla/mux"
	"github.com/samber/lo"

	"github.com/filecoin-project/go-address"
	"github.com/filecoin-project/go-bitfield"
	"github.com/filecoin-project/go-state-types/abi"
	"github.com/filecoin-project/go-state-types/big"
	"github.com/filecoin-project/go-state-types/builtin/v9/market"

	"github.com/filecoin-project/lotus/chain/actors/builtin/miner"
	"github.com/filecoin-project/lotus/chain/types"
	"github.com/filecoin-project/lotus/cli/spcli"
	"github.com/filecoin-project/lotus/cmd/curio/deps"
	"github.com/filecoin-project/lotus/curiosrc/web/api/apihelper"
	"github.com/filecoin-project/lotus/storage/sealer/storiface"
)

const verifiedPowerGainMul = 9

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
		IsFilPlus      bool
		SealInfo       string
		Proving        bool
		Flag           bool
		DealWeight     string
		Deals          string
		//StorageID         string         `db:"storage_id"` // map to serverName
		// Activation        abi.ChainEpoch // map to time.Time. advanced view only
		// DealIDs           []abi.DealID //  advanced view only
		//ExpectedDayReward abi.TokenAmount
		//SealProof         abi.RegisteredSealProof
	}

	type piece struct {
		Size     int64           `db:"piece_size"`
		DealID   uint64          `db:"f05_deal_id"`
		Proposal json.RawMessage `db:"f05_deal_proposal"`
		Manifest json.RawMessage `db:"direct_piece_activation_manifest"`
		Miner    int64           `db:"sp_id"`
		Sector   int64           `db:"sector_number"`
	}
	var sectors []sector
	var pieces []piece
	apihelper.OrHTTPFail(w, c.DB.Select(r.Context(), &sectors, `SELECT 
		miner_id, sector_num, SUM(sector_filetype) as sector_filetype  
		FROM sector_location WHERE sector_filetype != 32
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
		sectorIdx[sectorID{s.MinerID, uint64(s.SectorNum)}] = i
		if _, ok := minerToAddr[s.MinerID]; !ok {
			minerToAddr[s.MinerID], err = address.NewIDAddress(uint64(s.MinerID))
			apihelper.OrHTTPFail(w, err)
		}
	}

	// Get all pieces
	apihelper.OrHTTPFail(w, c.DB.Select(r.Context(), &pieces, `SELECT 
										sp_id,
										sector_number,
										piece_size,
										COALESCE(f05_deal_id, 0) AS f05_deal_id,
										f05_deal_proposal,
										direct_piece_activation_manifest  
										FROM sectors_sdr_initial_pieces 
										ORDER BY sp_id, sector_number`))
	pieceIndex := map[sectorID][]int{}
	for i, piece := range pieces {
		piece := piece
		cur := pieceIndex[sectorID{mID: piece.Miner, sNum: uint64(piece.Sector)}]
		pieceIndex[sectorID{mID: piece.Miner, sNum: uint64(piece.Sector)}] = append(cur, i)
	}

	for minerID, maddr := range minerToAddr {
		onChainInfo, err := c.getCachedSectorInfo(w, r, maddr, head.Key())
		apihelper.OrHTTPFail(w, err)
		for _, chainy := range onChainInfo {
			st := chainy.onChain
			if i, ok := sectorIdx[sectorID{minerID, uint64(st.SectorNumber)}]; ok {
				sectors[i].IsOnChain = true
				sectors[i].ExpiresAt = st.Expiration
				sectors[i].IsFilPlus = st.VerifiedDealWeight.GreaterThan(big.NewInt(0))
				if ss, err := st.SealProof.SectorSize(); err == nil {
					sectors[i].SealInfo = ss.ShortString()
				}
				sectors[i].Proving = chainy.active
				if st.Expiration < head.Height() {
					sectors[i].Flag = true // Flag expired sectors
				}

				dw, vp := .0, .0
				f05, ddo := 0, 0
				var pi []piece
				if j, ok := pieceIndex[sectorID{sectors[i].MinerID, uint64(sectors[i].SectorNum)}]; ok {
					for _, k := range j {
						pi = append(pi, pieces[k])
					}
				}
				estimate := st.Expiration-st.Activation <= 0 || sectors[i].HasSnap
				if estimate {
					for _, p := range pi {
						if p.Proposal != nil {
							var prop *market.DealProposal
							apihelper.OrHTTPFail(w, json.Unmarshal(p.Proposal, &prop))
							dw += float64(prop.PieceSize)
							if prop.VerifiedDeal {
								vp += float64(prop.PieceSize) * verifiedPowerGainMul
							}
							f05++
						}
						if p.Manifest != nil {
							var pam *miner.PieceActivationManifest
							apihelper.OrHTTPFail(w, json.Unmarshal(p.Manifest, &pam))
							dw += float64(pam.Size)
							if pam.VerifiedAllocationKey != nil {
								vp += float64(pam.Size) * verifiedPowerGainMul
							}
							ddo++
						}
					}
				} else {
					rdw := big.Add(st.DealWeight, st.VerifiedDealWeight)
					dw = float64(big.Div(rdw, big.NewInt(int64(st.Expiration-st.Activation))).Uint64())
					vp = float64(big.Div(big.Mul(st.VerifiedDealWeight, big.NewInt(verifiedPowerGainMul)), big.NewInt(int64(st.Expiration-st.Activation))).Uint64())
					// DDO sectors don't have deal info on chain
					for _, p := range pi {
						if p.Manifest != nil {
							ddo++
						}
						if p.Proposal != nil {
							f05++
						}
					}
				}
				sectors[i].DealWeight = "CC"
				if dw > 0 {
					sectors[i].DealWeight = fmt.Sprintf("%s", units.BytesSize(dw))
				}
				if vp > 0 {
					sectors[i].DealWeight = fmt.Sprintf("%s", units.BytesSize(vp))
				}
				sectors[i].Deals = fmt.Sprintf("Market: %d, DDO: %d", f05, ddo)
			} else {
				// sector is on chain but not in db
				s := sector{
					MinerID:   minerID,
					SectorNum: int64(chainy.onChain.SectorNumber),
					IsOnChain: true,
					ExpiresAt: chainy.onChain.Expiration,
					IsFilPlus: chainy.onChain.VerifiedDealWeight.GreaterThan(big.NewInt(0)),
					Proving:   chainy.active,
					Flag:      true, // All such sectors should be flagged to be terminated
				}
				if ss, err := chainy.onChain.SealProof.SectorSize(); err == nil {
					s.SealInfo = ss.ShortString()
				}
				sectors = append(sectors, s)
			}
			/*
				info, err := c.Full.StateSectorGetInfo(r.Context(), minerToAddr[s], abi.SectorNumber(uint64(sectors[i].SectorNum)), headKey)
				if err != nil {
					sectors[i].IsValid = false
					continue
				}*/
		}
	}

	// Add deal details to sectors which are not on chain
	for i := range sectors {
		if !sectors[i].IsOnChain {
			var pi []piece
			dw, vp := .0, .0
			f05, ddo := 0, 0

			// Find if there are any deals for this sector
			if j, ok := pieceIndex[sectorID{sectors[i].MinerID, uint64(sectors[i].SectorNum)}]; ok {
				for _, k := range j {
					pi = append(pi, pieces[k])
				}
			}

			if len(pi) > 0 {
				for _, p := range pi {
					if p.Proposal != nil {
						var prop *market.DealProposal
						apihelper.OrHTTPFail(w, json.Unmarshal(p.Proposal, &prop))
						dw += float64(prop.PieceSize)
						if prop.VerifiedDeal {
							vp += float64(prop.PieceSize) * verifiedPowerGainMul
						}
						f05++
					}
					if p.Manifest != nil {
						var pam *miner.PieceActivationManifest
						apihelper.OrHTTPFail(w, json.Unmarshal(p.Manifest, &pam))
						dw += float64(pam.Size)
						if pam.VerifiedAllocationKey != nil {
							vp += float64(pam.Size) * verifiedPowerGainMul
						}
						ddo++
					}
				}
			}
			sectors[i].IsFilPlus = vp > 0
			if dw > 0 {
				sectors[i].DealWeight = fmt.Sprintf("%s", units.BytesSize(dw))
			} else if vp > 0 {
				sectors[i].DealWeight = fmt.Sprintf("%s", units.BytesSize(vp))
			} else {
				sectors[i].DealWeight = "CC"
			}
			sectors[i].Deals = fmt.Sprintf("Market: %d, DDO: %d", f05, ddo)
		}
	}
	apihelper.OrHTTPFail(w, json.NewEncoder(w).Encode(map[string]any{"data": sectors}))
}

type sectorInfo struct {
	onChain *miner.SectorOnChainInfo
	active  bool
}

type sectorCacheEntry struct {
	sectors []sectorInfo
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
func (c *cfg) getCachedSectorInfo(w http.ResponseWriter, r *http.Request, maddr address.Address, headKey types.TipSetKey) ([]sectorInfo, error) {
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
		active, err := c.Full.StateMinerActiveSectors(context.Background(), maddr, headKey)
		if err != nil {
			mx.Lock()
			delete(sectorInfoCache, maddr)
			close(v.loading)
			mx.Unlock()
			return nil, err
		}
		activebf := bitfield.New()
		for i := range active {
			activebf.Set(uint64(active[i].SectorNumber))
		}
		infos := make([]sectorInfo, len(onChainInfo))
		for i, info := range onChainInfo {
			info := info
			set, err := activebf.IsSet(uint64(info.SectorNumber))
			if err != nil {
				mx.Lock()
				delete(sectorInfoCache, maddr)
				close(v.loading)
				mx.Unlock()
				return nil, err
			}
			infos[i] = sectorInfo{
				onChain: info,
				active:  set,
			}
		}
		mx.Lock()
		sectorInfoCache[maddr] = sectorCacheEntry{infos, nil, time.Now()}
		close(v.loading)
		mx.Unlock()
		return infos, nil
	}
	return v.sectors, nil
}

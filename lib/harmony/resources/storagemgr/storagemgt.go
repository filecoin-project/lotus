// Storagemgr is a utility for harmonytask tasks
// to manage the available space.
// Ex:
//
//	TaskTypeDefinition{
//	   Storage: storMgr.MakeFuncs(UsageTemporary, 1<<30, func(taskID int) string {
//					return fmt.Sprintf("task-%d", taskID)   // or use SQL to get the sector ID
//			}),
//	}
//
// Also useful: s.CombineFuncs(s1, s2, s3)
package storagemgr

import (
	"bytes"
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"sync"

	logging "github.com/ipfs/go-log/v2"
	"golang.org/x/xerrors"

	"github.com/filecoin-project/lotus/lib/harmony/resources"
	"github.com/filecoin-project/lotus/storage/sealer/storiface"
)

var log = logging.Logger("harmonytask")

type path string

var claimsMx sync.Mutex
var claims = map[path][]consumption{}

var purposes = map[Usage][]path{}

type Usage int

const (
	UsagePermanent = iota
	UsageTemporary
	UsageCache
	UsageStaging
)

type StorageMgr struct { // Functions are attached here to force users to call New()
}

func New(paths []storiface.StoragePath) *StorageMgr {
	// Note: this may be called multiple times per process.
	// populate purposes (global)
	for _, p := range paths {
		// @reviewer help this logic make sense
		var t Usage = UsagePermanent
		if p.CanSeal {
			t = UsageStaging
		}
		if strings.Contains(p.LocalPath, "cache") {
			t = UsageCache
		}
		if strings.Contains(p.LocalPath, "tmp") {
			t = UsageTemporary
		}
		purposes[t] = append(purposes[t], path(p.LocalPath))
	}

	return &StorageMgr{}
}

// For lotus-provider (but not other deps users).
// At start-up, remove (rm -rf) all the contents of all the claims.json files.
func (s *StorageMgr) Cleanup() {
	for _, paths := range purposes {
		for _, path := range paths {
			for _, u := range getJSON(path) {
				_ = os.RemoveAll(u.Location)
			}
			_ = os.Remove(string(path) + "/claims.json")
		}
	}
}

func (s *StorageMgr) MakeFuncs(purpose Usage, need uint64, namer func(taskID int) string) resources.Storage {
	return resources.Storage{
		HasCapacity: func() bool { _, ok := s.hasCapacity(purpose, need); return ok },
		Claim: func(id int) (string, error) {
			name := namer(id)
			return s.claim(purpose, need, name)
		},
		MarkComplete: s.markComplete,
	}
}

type consumption struct {
	Location string
	Need     uint64
}

func getJSON(path path) []consumption {
	b, err := os.ReadFile(string(path) + "/claims.json")
	if err != nil {
		log.Errorf("error reading claims: %s", err)
	} else {
		var consumages []consumption
		err := json.NewDecoder(bytes.NewReader(b)).Decode(&consumages)
		if err != nil {
			log.Errorf("error decoding claims: %s", err)
		} else {
			return consumages
		}
	}
	return nil
}

func (s *StorageMgr) hasCapacity(purpose Usage, need uint64) (path, bool) {
	for _, path := range purposes[purpose] {
		free, err := resources.DiskFree(string(path))
		if err != nil {
			log.Errorf("error checking free space: %s", err)
			continue
		}
		if free < need {
			continue
		}

		claimsMx.Lock()
		defer claimsMx.Unlock()
		consumages := getJSON(path)
		for _, c := range consumages {
			free -= c.Need // presume the whole need is used

			// no point in believing the errors on hot storage
			_ = filepath.Walk(c.Location, func(path string, info os.FileInfo, err error) error {
				if err != nil {
					return err
				}
				if info.IsDir() {
					return nil
				}
				free += uint64(info.Size()) // give more space available if it's already used
				return nil
			})

		}
		if free > need {
			return path, true
		}
	}
	return "", false
}
func (s *StorageMgr) claim(purpose Usage, need uint64, name string) (location string, err error) {
	path, ok := s.hasCapacity(purpose, need)
	if !ok {
		return "", xerrors.Errorf("no space for purpose %d", purpose)
	}

	consumption := consumption{Location: filepath.Join(string(path), name), Need: need}
	claimsMx.Lock()
	defer claimsMx.Unlock()

	j := getJSON(path)
	j = append(j, consumption)
	b, err := json.Marshal(j)
	if err != nil {
		return "", xerrors.Errorf("error encoding claims: %w", err)
	}
	err = os.WriteFile(string(path)+"/claims.json", b, 0644)
	if err != nil {
		return "", xerrors.Errorf("error writing claims: %w", err)
	}

	claims[path] = append(claims[path], consumption)
	return location, nil
}

func (s *StorageMgr) markComplete(location string) error {
	claimsMx.Lock()
	defer claimsMx.Unlock()

	foundPath := ""
	// Clean up RAM
	for path, consumages := range claims {
		if strings.HasPrefix(location, string(path)) {
			for i, c := range consumages {
				if c.Location == location {
					claims[path] = append(consumages[:i], consumages[i+1:]...)
					foundPath = string(path)
					break
				}
			}
		}
	}
	// erase the claim on disk
	if foundPath != "" {
		j := getJSON(path(foundPath))
		for i, c := range j {
			if c.Location == location {
				j = append(j[:i], j[i+1:]...)
				break
			}
		}
		b, err := json.Marshal(j)
		if err != nil {
			return xerrors.Errorf("error encoding claims: %w", err)
		}
		err = os.WriteFile(foundPath+"/claims.json", b, 0644)
		if err != nil {
			return xerrors.Errorf("error writing claims: %w", err)
		}
	}

	return nil
}

func (s *StorageMgr) CombineFuncs(ss ...resources.Storage) resources.Storage {
	return resources.Storage{
		HasCapacity: func() bool {
			for _, s := range ss {
				if !s.HasCapacity() {
					return false
				}
			}
			return true
		},
		Claim: func(id int) (string, error) {
			locations := make([]string, 0, len(ss))
			for _, s := range ss {
				location, err := s.Claim(id)
				if err != nil {
					for _, loc := range locations {
						_ = s.MarkComplete(loc)
					}
					return "", err
				}
				locations = append(locations, location)
			}

			return strings.Join(locations, ","), xerrors.Errorf("no space")
		},
		MarkComplete: func(locations string) error {
			var errKept error
			for _, location := range strings.Split(locations, ",") {
				if err := s.markComplete(location); err != nil {
					errKept = err
				}
			}
			return errKept
		},
	}
}

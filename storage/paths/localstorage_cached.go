package paths

import (
	"os"
	"sync"
	"time"

	lru "github.com/hashicorp/golang-lru/v2"

	"github.com/filecoin-project/lotus/storage/sealer/fsutil"
	"github.com/filecoin-project/lotus/storage/sealer/storiface"
)

var (
	StatTimeout          = 5 * time.Second
	MaxDiskUsageDuration = time.Second
)

type cachedLocalStorage struct {
	base LocalStorage

	statLk  sync.Mutex
	stats   *lru.Cache[string, statEntry]
	pathDUs *lru.Cache[string, *diskUsageEntry]
}

func newCachedLocalStorage(ls LocalStorage) *cachedLocalStorage {
	statCache, _ := lru.New[string, statEntry](1024)
	duCache, _ := lru.New[string, *diskUsageEntry](1024)

	return &cachedLocalStorage{
		base:    ls,
		stats:   statCache,
		pathDUs: duCache,
	}
}

type statEntry struct {
	stat fsutil.FsStat
	time time.Time
}

type diskUsageEntry struct {
	last diskUsageResult

	usagePromise <-chan diskUsageResult
}

type diskUsageResult struct {
	usage int64
	time  time.Time
}

func (c *cachedLocalStorage) GetStorage() (storiface.StorageConfig, error) {
	return c.base.GetStorage()
}

func (c *cachedLocalStorage) SetStorage(f func(*storiface.StorageConfig)) error {
	return c.base.SetStorage(f)
}

func (c *cachedLocalStorage) Stat(path string) (fsutil.FsStat, error) {
	c.statLk.Lock()
	defer c.statLk.Unlock()

	if v, ok := c.stats.Get(path); ok && time.Since(v.time) < StatTimeout {
		return v.stat, nil
	}

	// if we don't, get the stat
	st, err := c.base.Stat(path)
	if err == nil {
		c.stats.Add(path, statEntry{
			stat: st,
			time: time.Now(),
		})
	}

	return st, err
}

func (c *cachedLocalStorage) DiskUsage(path string) (int64, error) {
	c.statLk.Lock()
	defer c.statLk.Unlock()

	var entry *diskUsageEntry

	if v, ok := c.pathDUs.Get(path); ok {
		entry = v

		// if we have recent cached entry, use that
		if time.Since(entry.last.time) < StatTimeout {
			return entry.last.usage, nil
		}
	} else {
		entry = new(diskUsageEntry)
		c.pathDUs.Add(path, entry)
	}

	// start a new disk usage request; this can take a while so start a
	// goroutine, and if it doesn't return quickly, return either the
	// previous value, or zero - that's better than potentially blocking
	// here for a long time.
	if entry.usagePromise == nil {
		resCh := make(chan diskUsageResult, 1)
		go func() {
			du, err := c.base.DiskUsage(path)
			if err != nil {
				if !os.IsNotExist(err) {
					log.Errorw("error getting disk usage", "path", path, "error", err)
				}
			}
			resCh <- diskUsageResult{
				usage: du,
				time:  time.Now(),
			}
		}()
		entry.usagePromise = resCh
	}

	// wait for the disk usage result; if it doesn't come, fallback on
	// previous usage
	select {
	case du := <-entry.usagePromise:
		entry.usagePromise = nil
		entry.last = du
	case <-time.After(MaxDiskUsageDuration):
		log.Warnw("getting usage is slow, falling back to previous usage",
			"path", path,
			"fallback", entry.last.usage,
			"age", time.Since(entry.last.time))
	}

	return entry.last.usage, nil
}

var _ LocalStorage = &cachedLocalStorage{}

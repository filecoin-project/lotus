package shardedmutex

import (
	"fmt"
	"hash/maphash"
	"runtime"
	"sync"
	"sync/atomic"
	"testing"
	"time"
)

func TestLockingDifferentShardsDoesNotBlock(t *testing.T) {
	shards := 16
	sm := New(shards)
	done := make(chan struct{})
	go func() {
		select {
		case <-done:
			return
		case <-time.After(5 * time.Second):
			panic("test locked up")
		}
	}()
	for i := 0; i < shards; i++ {
		sm.Lock(i)
	}

	close(done)
}
func TestLockingSameShardsBlocks(t *testing.T) {
	shards := 16
	sm := New(shards)
	wg := sync.WaitGroup{}
	wg.Add(shards)
	ch := make(chan int, shards)

	for i := 0; i < shards; i++ {
		go func(i int) {
			if i != 15 {
				sm.Lock(i)
			}
			wg.Done()
			wg.Wait()
			sm.Lock((15 + i) % shards)
			ch <- i
			sm.Unlock(i)
		}(i)
	}

	wg.Wait()
	for i := 0; i < 2*shards; i++ {
		runtime.Gosched()
	}
	for i := 0; i < shards; i++ {
		if a := <-ch; a != i {
			t.Errorf("got %d instead of %d", a, i)
		}
	}
}

func TestShardedByString(t *testing.T) {
	shards := 16
	sm := NewFor(maphash.String, shards)

	wg1 := sync.WaitGroup{}
	wg1.Add(shards * 20)
	wg2 := sync.WaitGroup{}
	wg2.Add(shards * 20)

	active := atomic.Int32{}
	max := atomic.Int32{}

	for i := 0; i < shards*20; i++ {
		go func(i int) {
			wg1.Done()
			wg1.Wait()
			sm.Lock(fmt.Sprintf("goroutine %d", i))
			activeNew := active.Add(1)
			for {
				curMax := max.Load()
				if curMax >= activeNew {
					break
				}
				if max.CompareAndSwap(curMax, activeNew) {
					break
				}
			}
			for j := 0; j < 100; j++ {
				runtime.Gosched()
			}
			active.Add(-1)
			sm.Unlock(fmt.Sprintf("goroutine %d", i))
			wg2.Done()
		}(i)
	}

	wg2.Wait()

	if max.Load() != 16 {
		t.Fatal("max load not achieved", max.Load())
	}

}

func BenchmarkShardedMutex(b *testing.B) {
	shards := 16
	sm := New(shards)

	done := atomic.Int32{}
	go func() {
		for {
			sm.Lock(0)
			sm.Unlock(0)
			if done.Load() != 0 {
				return
			}
		}
	}()
	for i := 0; i < 100; i++ {
		runtime.Gosched()
	}

	b.ResetTimer()
	for b.Loop() {
		sm.Lock(1)
		sm.Unlock(1)
	}
	done.Add(1)
}

func BenchmarkShardedMutexOf(b *testing.B) {
	shards := 16
	sm := NewFor(maphash.String, shards)

	str1 := "string1"
	str2 := "string2"

	done := atomic.Int32{}
	go func() {
		for {
			sm.Lock(str1)
			sm.Unlock(str1)
			if done.Load() != 0 {
				return
			}
		}
	}()
	for i := 0; i < 100; i++ {
		runtime.Gosched()
	}

	b.ResetTimer()
	for b.Loop() {
		sm.Lock(str2)
		sm.Unlock(str2)
	}
	done.Add(1)
}

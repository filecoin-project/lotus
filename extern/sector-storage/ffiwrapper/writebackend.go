package ffiwrapper

import (
	"errors"
	"fmt"
	"io"
	"os"
	"sync"
	"syscall"
	"time"
)

type task struct {
	buf    []byte
	offset int64
	wg     *sync.WaitGroup
}

type processSize struct {
	len int64
	pos int64
}

func calculatePsize(srcSize, blockSize, parallel int64) ([]processSize, error) {
	if srcSize <= 0 || blockSize <= 0 || parallel <= 0 {
		return nil, errors.New("invalid argument")
	}
	blockCount := srcSize / blockSize
	if blockCount == 0 {
		return []processSize{{len: srcSize, pos: 0}}, nil
	}

	var psizes []processSize
	parallelBlockCount := blockCount / parallel
	blockCountMod := blockCount % parallel
	pos := int64(0)
	for i := int64(0); i < parallel; i++ {
		perBlockCount := parallelBlockCount
		if i < blockCountMod {
			perBlockCount++
		}
		psize := processSize{
			len: perBlockCount * blockSize,
			pos: pos,
		}
		pos += psize.len

		if psize.len != 0 {
			psizes = append(psizes, psize)
		}
	}
	if len(psizes) > 0 {
		lastPsize := psizes[len(psizes)-1]
		if srcSize%blockSize != 0 {
			lastPsize.len = lastPsize.len + srcSize%blockSize
		}
		psizes[len(psizes)-1] = lastPsize
	}
	return psizes, nil
}

// multiple parallel write from src to dst
// blockSize, copy size of each io
// parallel, num of parallel
func MultiWrite(src string, dst string, blockSize int64, parallel int64) error {
	// param check
	srcInfo, err := os.Stat(src)
	if nil != err {
		return err
	}
	srcSize := srcInfo.Size()
	if srcSize == 0 {
		return fmt.Errorf("src file '%s' size is 0", src)
	}

	if _, err := os.Stat(dst); nil == err {
		return errors.New("dst file is exist")
	} else if nil != err && !os.IsNotExist(err) {
		return err
	}
	if parallel < 0 {
		return errors.New("parallel must greater than 0")
	}
	if blockSize < 0 {
		return errors.New("blockSize must greater than 0")
	}

	fsrc, err := os.Open(src)
	if err != nil {
		return err
	}
	defer fsrc.Close()

	fdst, err := os.OpenFile(dst, os.O_RDWR|os.O_CREATE|os.O_TRUNC|syscall.O_DIRECT, 0666)
	if nil != err {
		return err
	}
	defer fdst.Close()

	// first, truncate dst with src file size
	if err = fdst.Truncate(srcSize); err != nil {
		log.Errorf("Truncate file %s failed, err %+v", dst, err)
		return err
	}

	psizes, err := calculatePsize(srcSize, blockSize, parallel)
	if err != nil {
		return err
	}

	// for read worker
	workWg := sync.WaitGroup{}
	// for write func
	writeWg := sync.WaitGroup{}
	// a chan for write op
	writeCh := make(chan *task, parallel)
	// a chan for stopping write scheduler
	stopCh := make(chan bool)
	errCh := make(chan error)

	// write scheduler
	go func() {
		writeLimit := make(chan struct{}, parallel)
		for {
			writeLimit <- struct{}{}
			select {
			case t := <-writeCh:
				go func() {
					err := t.write(fdst)
					if err != nil {
						errCh <- err
					}
					<-writeLimit
				}()
			case <-stopCh:
				return
			}
		}
	}()

	// read file worker
	worker := func(len, pos int64) error {
		defer workWg.Done()

		// size of have read
		rsize := int64(0)
		roffset := pos
		for {
			if len == rsize {
				break
			}
			buf := make([]byte, blockSize)
			rlen, err := fsrc.ReadAt(buf, roffset)
			if err == io.EOF {
				buf = buf[:rlen]
			} else if err != nil {
				log.Errorf("read file %s err: %+v", fsrc.Name(), err)
				return err
			}
			t := &task{
				buf:    buf,
				offset: roffset,
				wg:     &writeWg,
			}
			writeWg.Add(1)
			writeCh <- t

			rsize += int64(rlen)
			roffset += int64(rlen)
		}
		return nil
	}

	// have wrote size
	for _, p := range psizes {
		workWg.Add(1)
		go func(len, pos int64) {
			err := worker(len, pos)
			if err != nil {
				errCh <- err
			}
		}(p.len, p.pos)
	}

	doneCh := make(chan bool)
	go func() {
		workWg.Wait()
		writeWg.Wait()
		stopCh <- true
		doneCh <- true
	}()
	var lastErr error
LOOP:
	for {
		select {
		case lastErr = <-errCh:
			stopCh <- true
			break LOOP
		case <-doneCh:
			break LOOP
		}
	}
	return lastErr
}

// write file
func (t *task) write(f *os.File) (err error) {
	defer t.wg.Done()
	// retry 3 times, if WriteAt return err
	for i := 0; i < 3; i++ {
		if i != 0 {
			log.Errorf("sleep 10s, and retry writing %d time", i)
			time.Sleep(10 * time.Second)
		}
		_, err = f.WriteAt(t.buf, t.offset)
		if err == nil {
			break
		}
		log.Errorf("write file %s failed, err: %+v", f.Name(), err)
	}
	return err
}

// simplewrite
func SimpleWrite(src string, dst string) (int64, error) {
	srcSector, err := os.Open(src)
	if err != nil {
		return 0, err
	}
	dstSector, err := os.OpenFile(dst, os.O_WRONLY|os.O_CREATE, 0666)
	if err != nil {
		return 0, err
	}
	defer srcSector.Close()
	defer dstSector.Close()

	return io.Copy(dstSector, srcSector)
}

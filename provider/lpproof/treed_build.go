package lpproof

import (
	commcid "github.com/filecoin-project/go-fil-commcid"
	"github.com/filecoin-project/go-state-types/abi"
	"github.com/hashicorp/go-multierror"
	"github.com/ipfs/go-cid"
	"github.com/minio/sha256-simd"
	"golang.org/x/xerrors"
	"io"
	"math/bits"
	"os"
	"runtime"
	"sync"
	"time"
)

const nodeSize = 32
const threadChunkSize = 1 << 20
const nodesPerChunk = threadChunkSize / nodeSize

func hashChunk(data [][]byte) {
	l1Nodes := len(data[0]) / nodeSize / 2

	d := sha256.New()

	for i := 0; i < l1Nodes; i++ {
		levels := bits.TrailingZeros(^uint(i)) + 1

		inNode := i * 2 // at level 0
		outNode := i

		for l := 0; l < levels; l++ {
			d.Reset()
			inNodeData := data[l][inNode*nodeSize : (inNode+2)*nodeSize]
			d.Write(inNodeData)
			copy(data[l+1][outNode*nodeSize:(outNode+1)*nodeSize], d.Sum(nil))
			// set top bits to 00
			data[l+1][outNode*nodeSize+nodeSize-1] &= 0x3f

			inNode--
			inNode >>= 1
			outNode >>= 1
		}
	}
}

func BuildTreeD(data io.Reader, outPath string, size abi.PaddedPieceSize) (cid.Cid, error) {
	out, err := os.Create(outPath)
	if err != nil {
		return cid.Undef, err
	}

	outSize := treeSize(size)

	// allocate space for the tree
	err = out.Truncate(int64(outSize))
	if err != nil {
		return cid.Undef, err
	}

	// setup buffers
	maxThreads := int64(size) / threadChunkSize
	if maxThreads > int64(runtime.NumCPU()) {
		maxThreads = int64(runtime.NumCPU())
	}
	if maxThreads < 1 {
		maxThreads = 1
	}

	// allocate buffers
	var bufLk sync.Mutex
	workerBuffers := make([][][]byte, maxThreads) // [worker][level][levelSize]

	for i := range workerBuffers {
		workerBuffer := make([][]byte, 1)

		bottomBufSize := int64(threadChunkSize)
		if bottomBufSize > int64(size) {
			bottomBufSize = int64(size)
		}
		workerBuffer[0] = make([]byte, bottomBufSize)

		// append levels until we get to a 32 byte level
		for len(workerBuffer[len(workerBuffer)-1]) > 32 {
			newLevel := make([]byte, len(workerBuffer[len(workerBuffer)-1])/2)
			workerBuffer = append(workerBuffer, newLevel)
		}
		workerBuffers[i] = workerBuffer
	}

	// prepare apex buffer
	apexBottomSize := uint64(size) / uint64(len(workerBuffers[0][0]))
	var apexBuf [][]byte
	threadLayers := 1

	if apexBottomSize > 1 {
		apexBuf = make([][]byte, 1)
		apexBuf[0] = make([]byte, apexBottomSize)
		for len(apexBuf[len(apexBuf)-1]) > 32 {
			newLevel := make([]byte, len(apexBuf[len(apexBuf)-1])/2)
			apexBuf = append(apexBuf, newLevel)
			threadLayers++
		}
	}

	// start processing
	var processed uint64
	var workWg sync.WaitGroup
	var errLock sync.Mutex
	var oerr error

	for processed < uint64(size) {
		// get a buffer
		bufLk.Lock()
		if len(workerBuffers) == 0 {
			bufLk.Unlock()
			time.Sleep(50 * time.Microsecond)
			continue
		}

		// pop last
		workBuffer := workerBuffers[len(workerBuffers)-1]
		workerBuffers = workerBuffers[:len(workerBuffers)-1]

		bufLk.Unlock()

		// before reading check that we didn't get a write error
		errLock.Lock()
		if oerr != nil {
			errLock.Unlock()
			return cid.Undef, oerr
		}
		errLock.Unlock()

		// read data into the bottom level
		// note: the bottom level will never be too big; data is power of two
		// size, and if it's smaller than a single buffer, we only have one
		// smaller buffer

		_, err := io.ReadFull(data, workBuffer[0])
		if err != nil && err != io.EOF {
			return cid.Undef, err
		}

		// start processing
		workWg.Add(1)
		go func(startOffset uint64) {
			hashChunk(workBuffer)

			// persist apex if needed
			if len(apexBuf) > 0 {
				apexHash := workBuffer[len(workBuffer)-1]
				hashPos := startOffset >> threadLayers

				copy(apexBuf[0][hashPos:hashPos+nodeSize], apexHash)
			}

			// write results
			offsetInLayer := startOffset
			for layer, layerData := range workBuffer {

				// layerOff is outSize:bits[most significant bit - layer]
				layerOff := layerOffset(uint64(size), layer)
				dataOff := offsetInLayer + layerOff
				offsetInLayer /= 2

				_, werr := out.WriteAt(layerData, int64(dataOff))
				if werr != nil {
					errLock.Lock()
					oerr = multierror.Append(oerr, werr)
					errLock.Unlock()
					return
				}
			}

			// return buffer
			bufLk.Lock()
			workerBuffers = append(workerBuffers, workBuffer)
			bufLk.Unlock()

			workWg.Done()
		}(processed)

		processed += uint64(len(workBuffer[0]))
	}

	workWg.Wait()

	if oerr != nil {
		return cid.Undef, oerr
	}

	if len(apexBuf) > 0 {
		// hash the apex
		hashChunk(apexBuf)

		// write apex
		for apexLayer, layerData := range apexBuf {
			layer := apexLayer + threadLayers

			layerOff := layerOffset(uint64(size), layer)
			_, werr := out.WriteAt(layerData, int64(layerOff))
			if werr != nil {
				return cid.Undef, xerrors.Errorf("write apex: %w", werr)
			}
		}
	}

	var commp [32]byte
	if len(workerBuffers) == 1 {
		copy(commp[:], workerBuffers[0][0])
	} else {
		copy(commp[:], apexBuf[0])
	}

	commCid, err := commcid.DataCommitmentV1ToCID(commp[:])
	if err != nil {
		return cid.Undef, err
	}

	return commCid, nil
}

func treeSize(data abi.PaddedPieceSize) uint64 {
	bytesToAlloc := uint64(data)

	// append bytes until we get to nodeSize
	for todo := bytesToAlloc; todo > nodeSize; todo /= 2 {
		bytesToAlloc += todo / 2
	}

	return bytesToAlloc
}

func layerOffset(size uint64, layer int) uint64 {
	layerBits := uint64(1) << uint64(layer)
	layerBits--
	layerOff := (size * layerBits) >> uint64(layer-1)
	return layerOff
}

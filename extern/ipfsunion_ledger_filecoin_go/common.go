/*******************************************************************************
*   (c) 2018 ZondaX GmbH
*
*  Licensed under the Apache License, Version 2.0 (the "License");
*  you may not use this file except in compliance with the License.
*  You may obtain a copy of the License at
*
*      http://www.apache.org/licenses/LICENSE-2.0
*
*  Unless required by applicable law or agreed to in writing, software
*  distributed under the License is distributed on an "AS IS" BASIS,
*  WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
*  See the License for the specific language governing permissions and
*  limitations under the License.
********************************************************************************/

package ledger_filecoin_go

import (
	"encoding/binary"
	"fmt"
	"math"
)

const (
	userMessageChunkSize = 250
	publicKeyLength      = 65
)

// VersionInfo contains app version information
type VersionInfo struct {
	AppMode uint8
	Major   uint8
	Minor   uint8
	Patch   uint8
}

func (c VersionInfo) String() string {
	return fmt.Sprintf("%d.%d.%d", c.Major, c.Minor, c.Patch)
}

// VersionRequiredError the command is not supported by this app
type VersionRequiredError struct {
	Found    VersionInfo
	Required VersionInfo
}

func (e VersionRequiredError) Error() string {
	return fmt.Sprintf("App Version required %s - Version found: %s", e.Required, e.Found)
}

func NewVersionRequiredError(req VersionInfo, ver VersionInfo) error {
	return &VersionRequiredError{
		Found:    ver,
		Required: req,
	}
}

// CheckVersion compares the current version with the required version
func CheckVersion(ver VersionInfo, req VersionInfo) error {
	if ver.Major != req.Major {
		if ver.Major > req.Major {
			return nil
		}
		return NewVersionRequiredError(req, ver)
	}

	if ver.Minor != req.Minor {
		if ver.Minor > req.Minor {
			return nil
		}
		return NewVersionRequiredError(req, ver)
	}

	if ver.Patch >= req.Patch {
		return nil
	}
	return NewVersionRequiredError(req, ver)
}

func GetBip44bytes(bip44Path []uint32, hardenCount int) ([]byte, error) {
	message := make([]byte, 20)
	if len(bip44Path) != 5 {
		return nil, fmt.Errorf("path should contain 5 elements")
	}
	for index, element := range bip44Path {
		pos := index * 4
		value := element
		if index < hardenCount {
			value = 0x80000000 | element
		}
		binary.LittleEndian.PutUint32(message[pos:], value)
	}
	return message, nil
}

func prepareChunks(bip44PathBytes []byte, transaction []byte) ([][]byte, error) {
	var packetIndex = 0
	// first chunk + number of chunk needed for transaction
	var packetCount = 1 + int(math.Ceil(float64(len(transaction))/float64(userMessageChunkSize)))

	chunks := make([][]byte, packetCount)

	// First chunk is path
	chunks[0] = bip44PathBytes
	packetIndex++

	for packetIndex < packetCount {
		var start = (packetIndex - 1) * userMessageChunkSize
		var end = (packetIndex * userMessageChunkSize) - 1

		if end >= len(transaction) {
			chunks[packetIndex] = transaction[start:]
		} else {
			chunks[packetIndex] = transaction[start:end]
		}
		packetIndex++
	}

	return chunks, nil
}

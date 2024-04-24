/*******************************************************************************
*   (c) 2019 ZondaX GmbH
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
	"fmt"

	ledger_go "github.com/zondax/ledger-go"
)

const (
	CLA = 0x06

	INSGetVersion          = 0
	INSGetAddrSECP256K1    = 1
	INSSignSECP256K1       = 2
	INSSignSECP256K1NonMsg = 3 //ipfsunion add
)

const (
	PayloadChunkInit = 0
	PayloadChunkAdd  = 1
	PayloadChunkLast = 2
)

const HardenCount int = 2

// LedgerFilecoin represents a connection to the Ledger app
type LedgerFilecoin struct {
	api     ledger_go.LedgerDevice
	version VersionInfo
}

type SignatureAnswer struct {
	r            []byte
	s            []byte
	v            uint8
	derSignature []byte
}

func (sa *SignatureAnswer) SignatureBytes() []byte {
	out := make([]byte, 65)
	copy(out[:32], sa.r)
	copy(out[32:64], sa.s)
	out[64] = sa.v
	return out
}

// Displays existing Ledger Filecoin apps by address
func ListFilecoinDevices(path []uint32) {
	hid := ledger_go.NewLedgerAdmin()
	for i := 0; i < hid.CountDevices(); i++ {
		ledgerDevice, err := hid.Connect(i)
		if err != nil {
			continue
		}
		defer ledgerDevice.Close()

		app := LedgerFilecoin{ledgerDevice, VersionInfo{}}
		defer app.Close()

		appVersion, err := app.GetVersion()
		if err != nil {
			continue
		}

		_, _, addrString, err := app.GetAddressPubKeySECP256K1(path)
		if err != nil {
			continue
		}

		fmt.Printf("============ Device found\n")
		fmt.Printf("Filecoin App Version : %x\n", appVersion)
		fmt.Printf("Filecoin App Address : %s\n", addrString)
	}
}

// ConnectLedgerFilecoinApp connects to Filecoin app based on address
func ConnectLedgerFilecoinApp(seekingAddress string, path []uint32) (*LedgerFilecoin, error) {
	hid := ledger_go.NewLedgerAdmin()
	for i := 0; i < hid.CountDevices(); i += 1 {
		ledgerDevice, err := hid.Connect(i)
		if err != nil {
			continue
		}

		app := LedgerFilecoin{ledgerDevice, VersionInfo{}}
		_, _, addrString, err := app.GetAddressPubKeySECP256K1(path)
		if err != nil {
			defer app.Close()
			continue
		}
		if seekingAddress == "" || addrString == seekingAddress {
			return &app, nil
		}
	}
	return nil, fmt.Errorf("no Filecoin app with specified address found")
}

// FindLedgerFilecoinApp finds the Filecoin app running in a Ledger device
func FindLedgerFilecoinApp() (*LedgerFilecoin, error) {
	hid := ledger_go.NewLedgerAdmin()
	ledgerAPI, err := hid.Connect(0)

	if err != nil {
		return nil, err
	}

	app := LedgerFilecoin{ledgerAPI, VersionInfo{}}
	appVersion, err := app.GetVersion()

	if err != nil {
		defer ledgerAPI.Close()
		if err.Error() == "[APDU_CODE_CLA_NOT_SUPPORTED] Class not supported" {
			return nil, fmt.Errorf("are you sure the Filecoin app is open?")
		}
		return nil, err
	}

	err = app.CheckVersion(*appVersion)
	if err != nil {
		defer ledgerAPI.Close()
		return nil, err
	}

	return &app, err
}

// Close closes a connection with the Filecoin user app
func (ledger *LedgerFilecoin) Close() error {
	return ledger.api.Close()
}

// VersionIsSupported returns true if the App version is supported by this library
func (ledger *LedgerFilecoin) CheckVersion(ver VersionInfo) error {
	return CheckVersion(ver, VersionInfo{0, 0, 3, 0})
}

// GetVersion returns the current version of the Filecoin user app
func (ledger *LedgerFilecoin) GetVersion() (*VersionInfo, error) {
	message := []byte{CLA, INSGetVersion, 0, 0, 0}
	response, err := ledger.api.Exchange(message)

	if err != nil {
		return nil, err
	}

	if len(response) < 4 {
		return nil, fmt.Errorf("invalid response")
	}

	ledger.version = VersionInfo{
		AppMode: response[0],
		Major:   response[1],
		Minor:   response[2],
		Patch:   response[3],
	}

	return &ledger.version, nil
}

// SignSECP256K1 signs a transaction using Filecoin user app
// this command requires user confirmation in the device
func (ledger *LedgerFilecoin) SignSECP256K1(bip44Path []uint32, transaction []byte, isMsg bool) (*SignatureAnswer, error) { //ipfsunion add
	signatureBytes, err := ledger.sign(bip44Path, transaction, isMsg)
	if err != nil {
		return nil, err
	}

	// R,S,V and at least 1 bytes of the der sig
	if len(signatureBytes) < 66 {
		return nil, fmt.Errorf("The signature provided is too short.")
	}

	signatureAnswer := SignatureAnswer{
		signatureBytes[0:32],
		signatureBytes[32:64],
		signatureBytes[64],
		signatureBytes[65:]}

	return &signatureAnswer, nil
}

// GetPublicKeySECP256K1 retrieves the public key for the corresponding bip44 derivation path
// this command DOES NOT require user confirmation in the device
func (ledger *LedgerFilecoin) GetPublicKeySECP256K1(bip44Path []uint32) ([]byte, error) {
	pubkey, _, _, err := ledger.retrieveAddressPubKeySECP256K1(bip44Path, false)
	return pubkey, err
}

// GetAddressPubKeySECP256K1 returns the pubkey and addresses
// this command does not require user confirmation
func (ledger *LedgerFilecoin) GetAddressPubKeySECP256K1(bip44Path []uint32) (pubkey []byte, addrByte []byte, addrString string, err error) {
	return ledger.retrieveAddressPubKeySECP256K1(bip44Path, false)
}

// ShowAddressPubKeySECP256K1 returns the pubkey (compressed) and addresses
// this command requires user confirmation in the device
func (ledger *LedgerFilecoin) ShowAddressPubKeySECP256K1(bip44Path []uint32) (pubkey []byte, addrByte []byte, addrString string, err error) {
	return ledger.retrieveAddressPubKeySECP256K1(bip44Path, true)
}

func (ledger *LedgerFilecoin) GetBip44bytes(bip44Path []uint32, hardenCount int) ([]byte, error) {
	pathBytes, err := GetBip44bytes(bip44Path, hardenCount)
	if err != nil {
		return nil, err
	}

	return pathBytes, nil
}

func (ledger *LedgerFilecoin) sign(bip44Path []uint32, transaction []byte, isMsg bool) ([]byte, error) {//ipfsunion add

	pathBytes, err := ledger.GetBip44bytes(bip44Path, HardenCount)
	if err != nil {
		return nil, err
	}

	chunks, err := prepareChunks(pathBytes, transaction)
	if err != nil {
		return nil, err
	}

	var finalResponse []byte

	var message []byte

	var chunkIndex int = 0

	/*ipfsunion begin*/
	signType := byte(INSSignSECP256K1)
	if !isMsg {
		signType = byte(INSSignSECP256K1NonMsg)
	}
	/*ipfsunion end*/

	for chunkIndex < len(chunks) {
		payloadLen := byte(len(chunks[chunkIndex]))

		if chunkIndex == 0 {
			header := []byte{CLA, signType, PayloadChunkInit, 0, payloadLen}
			message = append(header, chunks[chunkIndex]...)
		} else {

			payloadDesc := byte(PayloadChunkAdd)
			if chunkIndex == (len(chunks) - 1) {
				payloadDesc = byte(PayloadChunkLast)
			}

			header := []byte{CLA, signType, payloadDesc, 0, payloadLen}
			message = append(header, chunks[chunkIndex]...)
		}

		response, err := ledger.api.Exchange(message)
		if err != nil {
			// FIXME: CBOR will be used instead
			if err.Error() == "[APDU_CODE_BAD_KEY_HANDLE] The parameters in the data field are incorrect" {
				// In this special case, we can extract additional info
				errorMsg := string(response)
				return nil, fmt.Errorf(errorMsg)
			}
			if err.Error() == "[APDU_CODE_DATA_INVALID] Referenced data reversibly blocked (invalidated)" {
				errorMsg := string(response)
				return nil, fmt.Errorf(errorMsg)
			}
			return nil, err
		}

		finalResponse = response
		chunkIndex++

	}
	return finalResponse, nil
}

// retrieveAddressPubKeySECP256K1 returns the pubkey and address
func (ledger *LedgerFilecoin) retrieveAddressPubKeySECP256K1(bip44Path []uint32, requireConfirmation bool) (pubkey []byte, addrByte []byte, addrString string, err error) {
	pathBytes, err := ledger.GetBip44bytes(bip44Path, HardenCount)
	if err != nil {
		return nil, nil, "", err
	}

	p1 := byte(0)
	if requireConfirmation {
		p1 = byte(1)
	}

	// Prepare message
	header := []byte{CLA, INSGetAddrSECP256K1, p1, 0, 0}
	message := append(header, pathBytes...)
	message[4] = byte(len(message) - len(header)) // update length

	response, err := ledger.api.Exchange(message)

	if err != nil {
		return nil, nil, "", err
	}
	if len(response) < 39 {
		return nil, nil, "", fmt.Errorf("Invalid response")
	}

	cursor := 0

	// Read pubkey
	pubkey = response[cursor:publicKeyLength]
	cursor = cursor + publicKeyLength

	// Read addr byte format length
	addrByteLength := int(response[cursor])
	cursor = cursor + 1

	// Read addr byte format
	addrByte = response[cursor : cursor+addrByteLength]
	cursor = cursor + addrByteLength

	// Read addr strin format length
	addrStringLength := int(response[cursor])
	cursor = cursor + 1

	// Read addr string format
	addrString = string(response[cursor : cursor+addrStringLength])

	return pubkey, addrByte, addrString, err
}

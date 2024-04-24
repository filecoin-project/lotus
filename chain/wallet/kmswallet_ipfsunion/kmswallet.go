package kmswallet

import (
	"context"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"net"
	"net/url"
	"os"
	"strings"
	"time"

	logging "github.com/ipfs/go-log"

	"github.com/filecoin-project/go-address"
	"github.com/filecoin-project/go-state-types/crypto"
	"github.com/filecoin-project/lotus/api"
	"github.com/filecoin-project/lotus/chain/types"
	vapi "github.com/hashicorp/vault/api"
	"github.com/hashicorp/vault/sdk/helper/jsonutil"
)

var log = logging.Logger("wallet-kms")

type KMSWalletConfig struct {
	Address          string
	AgentAddress     string
	OutputCurlString bool
	CACert           string
	CAPath           string
	ClientCert       string
	ClientKey        string
	TLSServerName    string
	TLSSkipVerify    bool
	MFAcreds         []string
	NS               string
	PolicyOverride   bool
	APIToken         string
}

type kmsClient struct {
	config *KMSWalletConfig
	client *vapi.Client
}

type KMSWallet struct {
	nodeName   string
	kmsClients []*kmsClient
}

func NewKMSWallet(serverURL string, nodeName string, apiToken string, skipTLSVerify bool) *KMSWallet {
	kmsW := &KMSWallet{}
	kmsW.nodeName = nodeName

	serverEndPoints := strings.Split(serverURL, ",")
	for _, serverEndPoint := range serverEndPoints {
		kmsConfig := &KMSWalletConfig{Address: serverEndPoint, APIToken: apiToken, TLSSkipVerify: skipTLSVerify}
		kmsC, err := kmsW.Client(kmsConfig)
		if err != nil || kmsC == nil {
			log.Errorf(fmt.Sprintf("can't create KMSWallet, please make sure that your kms client configuration is ok!, %s, %v", serverEndPoint, err))
			continue
		}

		kc := &kmsClient{
			kmsConfig,
			kmsC,
		}

		kmsW.kmsClients = append(kmsW.kmsClients, kc)
	}

	return kmsW
}

func (kw *KMSWallet) Client(kmsConfig *KMSWalletConfig) (*vapi.Client, error) {
	config := vapi.DefaultConfig()

	/*
		if err := config.ReadEnvironment(); err != nil {
			return nil, fmt.Errorf("failed to read environment: %v", err)
		}*/

	if kmsConfig.Address != "" {
		config.Address = kmsConfig.Address
	}
	if kmsConfig.AgentAddress != "" {
		config.Address = kmsConfig.AgentAddress
	}

	if kmsConfig.OutputCurlString {
		config.OutputCurlString = kmsConfig.OutputCurlString
	}

	// If we need custom TLS configuration, then set it
	if kmsConfig.CACert != "" || kmsConfig.CAPath != "" || kmsConfig.ClientCert != "" ||
		kmsConfig.ClientKey != "" || kmsConfig.TLSServerName != "" || kmsConfig.TLSSkipVerify {
		t := &vapi.TLSConfig{
			CACert:        kmsConfig.CACert,
			CAPath:        kmsConfig.CAPath,
			ClientCert:    kmsConfig.ClientCert,
			ClientKey:     kmsConfig.ClientKey,
			TLSServerName: kmsConfig.TLSServerName,
			Insecure:      kmsConfig.TLSSkipVerify,
		}

		// Setup TLS config
		if err := config.ConfigureTLS(t); err != nil {
			return nil, fmt.Errorf("failed to setup TLS config: %v", err)
		}
	}

	// Build the client
	client, err := vapi.NewClient(config)
	if err != nil {
		return nil, fmt.Errorf("failed to create client", err)
	}

	// Turn off retries on the CLI
	if os.Getenv(vapi.EnvVaultMaxRetries) == "" {
		client.SetMaxRetries(0)
	}

	// Set the wrapping function
	client.SetWrappingLookupFunc(vapi.DefaultWrappingLookupFunc)

	if kmsConfig.APIToken != "" {
		client.SetToken(kmsConfig.APIToken)
	} else {
		return nil, errors.New("blank token")
	}

	client.SetMFACreds(kmsConfig.MFAcreds)

	if kmsConfig.APIToken != "" {
		client.SetToken(kmsConfig.APIToken)
	}

	if kmsConfig.NS != "" {
		client.SetNamespace(kmsConfig.NS)
	}
	if kmsConfig.PolicyOverride {
		client.SetPolicyOverride(kmsConfig.PolicyOverride)
	}

	return client, nil
}

func (kw *KMSWallet) getAvailClient() *vapi.Client {
	timeout := 5 * time.Second

	for _, kc := range kw.kmsClients {
		u, err := url.Parse(kc.config.Address)
		if err != nil {
			continue
		}
		_, err = net.DialTimeout("tcp", u.Host, timeout)
		if err != nil {
			log.Errorf("Server %s unreachable, error: ", u.Host, err)
			continue
		}

		return kc.client
	}

	return nil
}

func (kw *KMSWallet) WalletNew(ctx context.Context, t types.KeyType) (address.Address, error) {
	if t != types.KTBLS && t != types.KTSecp256k1 {
		return address.Undef, fmt.Errorf("KMSWallet unsupported key type: '%s', only '%s' or '%s' of supported", t, types.KTBLS, types.KTSecp256k1)
	}

	path := "filecoin-vault/accounts/admin/" + kw.nodeName

	parameters := map[string]interface{}{
		"node_name": kw.nodeName,
		"key_type":  string(t),
	}

	kc := kw.getAvailClient()
	if kc == nil {
		log.Error("no avail kms server")
		return address.Undef, errors.New("no avail kms server")
	}
	sec, err := kc.Logical().Write(path, parameters)
	if err != nil {
		return address.Undef, fmt.Errorf("KMSWallet WalletNew create wallet err: %v", err)
	}

	if addr, ok := sec.Data["address"]; ok {
		return address.NewFromString(addr.(string))
	}

	return address.Undef, fmt.Errorf("KMSWallet WalletNew  can't create wallet")
}

func (kw *KMSWallet) WalletHas(ctx context.Context, addr address.Address) (bool, error) {
	path := "filecoin-vault/accounts/admin/" + kw.nodeName

	parameters := map[string][]string{
		"node_name": {kw.nodeName},
		"address":   {addr.String()},
	}

	kc := kw.getAvailClient()
	if kc == nil {
		log.Error("no avail kms server")
		return false, errors.New("no avail kms server")
	}
	sec, err := kc.Logical().ReadWithData(path, parameters)
	if err != nil {
		return false, fmt.Errorf("KMSWallet WalletHas read err: %v", err)
	}

	if _, ok := sec.Data["address"]; ok {
		return true, nil
	}

	return false, nil
}

func (kw *KMSWallet) WalletDelete(ctx context.Context, addr address.Address) error {
	path := "filecoin-vault/accounts/admin/" + kw.nodeName

	parameters := map[string][]string{
		"node_name": {kw.nodeName},
		"address":   {addr.String()},
	}

	kc := kw.getAvailClient()
	if kc == nil {
		log.Error("no avail kms server")
		return errors.New("no avail kms server")
	}
	_, err := kc.Logical().DeleteWithData(path, parameters)
	if err != nil {
		return fmt.Errorf("KMSWallet WalletDelete delete err: %v", err)
	}

	return nil
}

func (kw *KMSWallet) WalletList(ctx context.Context) ([]address.Address, error) {
	path := "filecoin-vault/accounts/list/" + kw.nodeName

	parameters := map[string][]string{
		"node_name": {kw.nodeName},
	}

	kc := kw.getAvailClient()
	if kc == nil {
		log.Error("no avail kms server")
		return nil, errors.New("no avail kms server")
	}
	sec, err := kc.Logical().ReadWithData(path, parameters)
	if err != nil {
		return nil, fmt.Errorf("KMSWallet WalletList list err: %v", err)
	}

	if sec == nil {
		return nil, fmt.Errorf("KMSWallet WalletList list sec nil: path=%s", path)
	}

	if value, ok := sec.Data["keys"]; ok {
		var addrs []address.Address

		if valArr, ok := value.([]interface{}); ok {
			for i := 0; i < len(valArr); i++ {
				addr, _ := address.NewFromString(valArr[i].(string))
				addrs = append(addrs, addr)
			}
		}

		return addrs, nil
	}

	return nil, nil
}

func (kw *KMSWallet) WalletImport(ctx context.Context, ki *types.KeyInfo) (address.Address, error) {
	path := "filecoin-vault/accounts/import/" + kw.nodeName

	b, err := json.Marshal(ki)
	if err != nil {
		return address.Undef, err
	}

	parameters := map[string]interface{}{
		"hex_key": hex.EncodeToString(b),
	}

	kc := kw.getAvailClient()
	if kc == nil {
		log.Error("no avail kms server")
		return address.Undef, errors.New("no avail kms server")
	}
	sec, err := kc.Logical().Write(path, parameters)
	if err != nil {
		return address.Undef, fmt.Errorf("KMSWallet WalletImport import err: %v", err)
	}

	if addr, ok := sec.Data["address"]; ok {
		address, _ := address.NewFromString(addr.(string))
		return address, nil
	}

	return address.Undef, errors.New("KMSWallet WalletImport invalid resp from KMS")
}

func (kw *KMSWallet) WalletExport(ctx context.Context, addr address.Address) (*types.KeyInfo, error) {
	path := "filecoin-vault/accounts/export/" + kw.nodeName

	parameters := map[string][]string{
		"address": {addr.String()},
	}

	kc := kw.getAvailClient()
	if kc == nil {
		log.Error("no avail kms server")
		return nil, errors.New("no avail kms server")
	}
	sec, err := kc.Logical().ReadWithData(path, parameters)
	if err != nil {
		return nil, fmt.Errorf("KMSWallet WalletExport read err: %v", err)
	}

	if hexKey, ok := sec.Data["hex_key"]; ok {
		keyBytes, err := hex.DecodeString(hexKey.(string))
		if err != nil {
			return nil, fmt.Errorf("KMSWallet WalletExport invalid key data: %v", err)
		}

		var keyInfo types.KeyInfo
		err = jsonutil.DecodeJSON(keyBytes, &keyInfo)
		if err != nil {
			return nil, fmt.Errorf("KMSWallet WalletExport invalid key data, DecodeJSON err: %v", err)
		}

		return &keyInfo, nil
	}

	return nil, fmt.Errorf("KMSWallet WalletExport can't find key: %s", addr.String())
}

func (kw *KMSWallet) WalletSign(ctx context.Context, addr address.Address, msg []byte, meta api.MsgMeta) (*crypto.Signature, error) {
	path := "filecoin-vault/accounts/sign/" + kw.nodeName

	metaBytes, err := json.Marshal(&meta)
	if err != nil {
		return nil, fmt.Errorf("invalid MsgMeta: %v", metaBytes)
	}

	parameters := map[string]interface{}{
		"address":    addr.String(),
		"encode_msg": hex.EncodeToString(msg),
		"meta_msg":   hex.EncodeToString(metaBytes),
	}

	kc := kw.getAvailClient()
	if kc == nil {
		log.Error("no avail kms server")
		return nil, errors.New("no avail kms server")
	}
	sec, err := kc.Logical().Write(path, parameters)
	if err != nil {
		return nil, fmt.Errorf("KMSWallet WalletSign sign err: %v", err)
	}

	if signedDataHex, ok := sec.Data["signed_data"]; ok {
		signedDataBytes, err := hex.DecodeString(signedDataHex.(string))
		if err != nil {
			return nil, fmt.Errorf("KMSWallet WalletSign invalid signed data: %v", err)
		}

		var signInfo crypto.Signature
		err = jsonutil.DecodeJSON(signedDataBytes, &signInfo)
		if err != nil {
			return nil, fmt.Errorf("KMSWallet WalletSign invalid signed data, DecodeJSON err: %v", err)
		}

		return &signInfo, nil
	}

	return nil, errors.New("can't get the signed data from kms")
}

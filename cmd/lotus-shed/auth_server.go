package main

import (
	"context"
	"encoding/base64"
	"fmt"
	"io"
	"net/http"
	"strconv"
	"strings"
	"time"

	"github.com/filecoin-project/go-address"
	"github.com/filecoin-project/go-state-types/crypto"
	lapi "github.com/filecoin-project/lotus/api"
	"github.com/filecoin-project/lotus/chain/types"
	lcli "github.com/filecoin-project/lotus/cli"
	"github.com/filecoin-project/lotus/lib/sigs"
	"github.com/urfave/cli/v2"
)

const (
	serviceName = "filecoin-auth-demo"
	maxAge      = 5 * time.Minute
)

var authServerStartCmd = &cli.Command{
	Name: "auth-server-start",
	Action: func(cctx *cli.Context) error {
		api, closer, err := lcli.GetFullNodeAPIV1(cctx)
		if err != nil {
			return err
		}
		defer closer()

		ctx := lcli.ReqContext(cctx)

		version, err := api.Version(ctx)
		if err != nil {
			return fmt.Errorf("failed to get API version: %w", err)
		}
		fmt.Printf("Connected to Lotus node: %s\n", version.Version)

		port := ":8080"
		if p := cctx.String("port"); p != "" {
			port = ":" + p
		}
		fmt.Printf("Starting auth server, connect to http://localhost%s/auth\n", port)

		server := &AuthServer{api: api}
		http.HandleFunc("/auth", server.handleAuth)
		return http.ListenAndServe(port, nil)
	},
}

type AuthServer struct {
	api lapi.FullNode
}

// ParseFilecoinAuth parses the authorization header
// Format: FilecoinSig address=<addr>;timestamp=<ts>;nonce=<nonce>;signature=<sig>
func ParseFilecoinAuth(authHeader string) (*AuthRequest, error) {
	if !strings.HasPrefix(authHeader, "FilecoinSig ") {
		return nil, fmt.Errorf("invalid auth scheme")
	}

	parts := strings.Split(strings.TrimPrefix(authHeader, "FilecoinSig "), ";")
	if len(parts) != 4 {
		return nil, fmt.Errorf("invalid auth format")
	}

	var auth AuthRequest
	for _, part := range parts {
		kv := strings.SplitN(part, "=", 2)
		if len(kv) != 2 {
			return nil, fmt.Errorf("invalid key-value pair")
		}

		switch kv[0] {
		case "address":
			addr, err := address.NewFromString(kv[1])
			if err != nil {
				return nil, fmt.Errorf("invalid address: %w", err)
			}
			auth.Address = addr

		case "timestamp":
			ts, err := strconv.ParseInt(kv[1], 10, 64)
			if err != nil {
				return nil, fmt.Errorf("invalid timestamp: %w", err)
			}
			auth.Timestamp = ts

		case "nonce":
			auth.Nonce = kv[1]

		case "signature":
			sigData, err := base64.StdEncoding.DecodeString(kv[1])
			if err != nil {
				return nil, fmt.Errorf("invalid signature encoding: %w", err)
			}

			if len(sigData) < 2 {
				return nil, fmt.Errorf("signature too short")
			}

			auth.Signature = crypto.Signature{
				Type: crypto.SigType(sigData[0]),
				Data: sigData[1:],
			}
		}
	}

	return &auth, nil
}

// AuthRequest represents a parsed authorization header
type AuthRequest struct {
	Address   address.Address
	Timestamp int64
	Nonce     string
	Signature crypto.Signature
}

func (s *AuthServer) validateAuth(ctx context.Context, authHeader string) (*AuthRequest, error) {
	auth, err := ParseFilecoinAuth(authHeader)
	if err != nil {
		return nil, fmt.Errorf("parse auth header: %w", err)
	}

	// Check timestamp age
	age := time.Since(time.Unix(auth.Timestamp, 0))
	if age > maxAge || age < -30*time.Second { // Allow 30s clock skew
		return nil, fmt.Errorf("timestamp too old or in future: %v", age)
	}

	// Reconstruct signed message
	message := fmt.Sprintf("AUTH:%s:%d:%s", serviceName, auth.Timestamp, auth.Nonce)

	// A lot below this line could be memoised in an LRU cache to avoid repeated lookups

	// Get the account key for the address; this resolves ID addresses to their public key address
	accountKey, err := s.api.StateAccountKey(ctx, auth.Address, types.EmptyTSK)
	if err != nil {
		// Try the original address if StateAccountKey fails?
		// What to do for delegated addresses? Will this work?
		accountKey = auth.Address
		fmt.Printf("StateAccountKey failed for %s, using original address: %v", auth.Address, err)
	}

	// Verify signature
	if err := sigs.Verify(&auth.Signature, accountKey, []byte(message)); err != nil {
		return nil, fmt.Errorf("signature verification failed: %w", err)
	}

	// Additional check: verify the address exists on chain, we would do additional steps here to
	// confirm that the address has set up payment, or whatever is needed for the service.
	_, err = s.api.StateLookupID(ctx, auth.Address, types.EmptyTSK)
	if err != nil {
		return nil, fmt.Errorf("address not found on chain: %w", err)
	}

	return auth, nil
}

func (s *AuthServer) handleAuth(w http.ResponseWriter, r *http.Request) {
	authHeader := r.Header.Get("Authorization")
	if authHeader == "" {
		http.Error(w, "missing authorization header", http.StatusUnauthorized)
		return
	}

	auth, err := s.validateAuth(r.Context(), authHeader)
	if err != nil {
		http.Error(w, fmt.Sprintf("authorization failed: %v", err), http.StatusUnauthorized)
		return
	}

	w.WriteHeader(http.StatusOK)
	fmt.Fprintf(w, "Authentication successful! Welcome %s\n", auth.Address)
}

var authClientRunCmd = &cli.Command{
	Name: "auth-client-run",
	Action: func(cctx *cli.Context) error {
		api, closer, err := lcli.GetFullNodeAPIV1(cctx)
		if err != nil {
			return err
		}
		defer closer()

		ctx := lcli.ReqContext(cctx)

		if cctx.NArg() < 1 {
			return fmt.Errorf("usage: auth-client-run <server-url>")
		}
		serverURL := cctx.Args().Get(0)
		if !strings.HasPrefix(serverURL, "http://") && !strings.HasPrefix(serverURL, "https://") {
			return fmt.Errorf("server URL must start with http:// or https://")
		}
		fmt.Printf("Connecting to server: %s\n", serverURL)

		// Get default wallet address
		walletAddr, err := api.WalletDefaultAddress(ctx)
		if err != nil {
			log.Fatalf("Failed to get default wallet address: %v", err)
		}
		fmt.Printf("Using wallet address: %s\n", walletAddr)

		// Check wallet balance (optional, just for info)
		balance, err := api.WalletBalance(ctx, walletAddr)
		if err != nil {
			fmt.Printf("Warning: Failed to get wallet balance: %v\n", err)
		} else {
			fmt.Printf("Wallet balance: %s\n", types.FIL(balance))
		}

		// Create auth header
		authHeader, err := CreateAuthHeader(api, walletAddr, serviceName)
		if err != nil {
			return fmt.Errorf("failed to create auth header: %w", err)
		}

		fmt.Printf("Created auth header: %s\n", authHeader)

		// Make HTTP request
		req, err := http.NewRequest("GET", serverURL, nil)
		if err != nil {
			log.Fatalf("Failed to create request: %v", err)
		}

		req.Header.Set("Authorization", authHeader)

		// Send request
		client := &http.Client{Timeout: 10 * time.Second}
		resp, err := client.Do(req)
		if err != nil {
			log.Fatalf("Request failed: %v", err)
		}
		defer resp.Body.Close()

		// Read response
		body, err := io.ReadAll(resp.Body)
		if err != nil {
			log.Fatalf("Failed to read response: %v", err)
		}

		fmt.Printf("Response status: %s\n", resp.Status)
		fmt.Printf("Response body: %s\n", string(body))

		if resp.StatusCode == http.StatusOK {
			fmt.Println("✅ Authentication successful!")
		} else {
			fmt.Println("❌ Authentication failed!")
		}

		return nil
	},
}

// CreateAuthHeader creates an authorization header for the given wallet address
func CreateAuthHeader(api lapi.FullNode, walletAddr address.Address, serviceName string) (string, error) {
	ctx := context.Background()

	timestamp := time.Now().Unix()
	nonce := fmt.Sprintf("%d", time.Now().UnixNano()) // Simple nonce

	// Create message
	message := fmt.Sprintf("AUTH:%s:%d:%s", serviceName, timestamp, nonce)

	// Use WalletSign to sign arbitrary data
	sig, err := api.WalletSign(ctx, walletAddr, []byte(message))
	if err != nil {
		return "", fmt.Errorf("failed to sign message: %w", err)
	}

	// Encode signature with type prefix
	sigData := append([]byte{byte(sig.Type)}, sig.Data...)
	sigB64 := base64.StdEncoding.EncodeToString(sigData)

	// Build header
	header := fmt.Sprintf("FilecoinSig address=%s;timestamp=%d;nonce=%s;signature=%s",
		walletAddr.String(), timestamp, nonce, sigB64)

	return header, nil
}

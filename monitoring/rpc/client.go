// package rpc provides a minimal JSON-RPC 2.0 client for querying
// goat (EVM-compatible) node endpoints.
package rpc

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"math/big"
	"net/http"
	"time"
)

// Client is a JSON-RPC client for an EVM-compatible node.
type Client struct {
	endpoint   string
	httpClient *http.Client
}

// SyncProgress holds the sync status fields returned by eth_syncing.
type SyncProgress struct {
	StartingBlock uint64 `json:"startingBlock"`
	CurrentBlock  uint64 `json:"currentBlock"`
	HighestBlock  uint64 `json:"highestBlock"`
}

// jsonRPCRequest represents a JSON-RPC 2.0 request.
type jsonRPCRequest struct {
	JSONRPC string        `json:"jsonrpc"`
	Method  string        `json:"method"`
	Params  []interface{} `json:"params"`
	ID      int           `json:"id"`
}

// jsonRPCResponse represents a JSON-RPC 2.0 response.
type jsonRPCResponse struct {
	JSONRPC string          `json:"jsonrpc"`
	Result  json.RawMessage `json:"result"`
	Error   *jsonRPCError   `json:"error,omitempty"`
	ID      int             `json:"id"`
}

// jsonRPCError represents a JSON-RPC 2.0 error object.
type jsonRPCError struct {
	Code    int    `json:"code"`
	Message string `json:"message"`
}

// NewClient creates a new RPC client for the given endpoint URL.
func NewClient(endpoint string) *Client {
	return &Client{
		endpoint: endpoint,
		httpClient: &http.Client{
			Timeout: 10 * time.Second,
		},
	}
}

// call executes a JSON-RPC method and returns the raw result.
func (c *Client) call(method string, params ...interface{}) (json.RawMessage, error) {
	if params == nil {
		params = []interface{}{}
	}

	req := jsonRPCRequest{
		JSONRPC: "2.0",
		Method:  method,
		Params:  params,
		ID:      1,
	}

	body, err := json.Marshal(req)
	if err != nil {
		return nil, fmt.Errorf("marshal request: %w", err)
	}

	resp, err := c.httpClient.Post(c.endpoint, "application/json", bytes.NewReader(body))
	if err != nil {
		return nil, fmt.Errorf("RPC request to %s: %w", c.endpoint, err)
	}
	defer resp.Body.Close()

	respBody, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, fmt.Errorf("read response body: %w", err)
	}

	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("RPC returned HTTP %d: %s", resp.StatusCode, string(respBody))
	}

	var rpcResp jsonRPCResponse
	if err := json.Unmarshal(respBody, &rpcResp); err != nil {
		return nil, fmt.Errorf("unmarshal response: %w", err)
	}

	if rpcResp.Error != nil {
		return nil, fmt.Errorf("RPC error %d: %s", rpcResp.Error.Code, rpcResp.Error.Message)
	}

	return rpcResp.Result, nil
}

// GetBlockNumber returns the current block height (eth_blockNumber).
func (c *Client) GetBlockNumber() (uint64, error) {
	result, err := c.call("eth_blockNumber")
	if err != nil {
		return 0, err
	}

	var hexBlock string
	if err := json.Unmarshal(result, &hexBlock); err != nil {
		return 0, fmt.Errorf("unmarshal block number: %w", err)
	}

	return parseHexUint64(hexBlock)
}

// GetChainID returns the chain ID (eth_chainId).
func (c *Client) GetChainID() (uint64, error) {
	result, err := c.call("eth_chainId")
	if err != nil {
		return 0, err
	}

	var hexChainID string
	if err := json.Unmarshal(result, &hexChainID); err != nil {
		return 0, fmt.Errorf("unmarshal chain id: %w", err)
	}

	return parseHexUint64(hexChainID)
}

// GetSyncStatus returns whether the node is syncing and its progress.
// if the node is fully synced, syncing=false and progress=nil.
// if the node is syncing, syncing=true and progress contains the details.
func (c *Client) GetSyncStatus() (bool, *SyncProgress, error) {
	result, err := c.call("eth_syncing")
	if err != nil {
		return false, nil, err
	}

	// eth_syncing returns `false` when not syncing, or an object when syncing
	var syncing bool
	if err := json.Unmarshal(result, &syncing); err == nil {
		// successfully parsed as boolean — node is not syncing
		return false, nil, nil
	}

	// parse as sync progress object
	var rawProgress map[string]string
	if err := json.Unmarshal(result, &rawProgress); err != nil {
		return false, nil, fmt.Errorf("unmarshal sync progress: %w", err)
	}

	progress := &SyncProgress{}
	if v, ok := rawProgress["startingBlock"]; ok {
		progress.StartingBlock, _ = parseHexUint64(v)
	}
	if v, ok := rawProgress["currentBlock"]; ok {
		progress.CurrentBlock, _ = parseHexUint64(v)
	}
	if v, ok := rawProgress["highestBlock"]; ok {
		progress.HighestBlock, _ = parseHexUint64(v)
	}

	return true, progress, nil
}

// parseHexUint64 converts a hex string (0x-prefixed) to uint64.
func parseHexUint64(hex string) (uint64, error) {
	n := new(big.Int)
	if _, ok := n.SetString(stripHexPrefix(hex), 16); !ok {
		return 0, fmt.Errorf("invalid hex value: %s", hex)
	}
	return n.Uint64(), nil
}

// stripHexPrefix removes the "0x" or "0X" prefix from a hex string.
func stripHexPrefix(s string) string {
	if len(s) >= 2 && (s[:2] == "0x" || s[:2] == "0X") {
		return s[2:]
	}
	return s
}

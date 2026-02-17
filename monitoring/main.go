// goat-monitor is a Prometheus exporter and health dashboard for
// monitoring a goat (EVM-compatible) RPC node.
//
// it tracks:
//   - current block height (eth_blockNumber)
//   - chain ID (eth_chainId)
//   - syncing status (eth_syncing)
//
// endpoints:
//
//	GET /metrics — Prometheus scrape endpoint
//	GET /health  — JSON health dashboard
//	GET /        — redirects to /health
package main

import (
	"encoding/json"
	"fmt"
	"log"
	"net/http"
	"os"
	"time"

	"github.com/layerzero-sre/goat-monitor/collector"
	"github.com/layerzero-sre/goat-monitor/rpc"
	"github.com/prometheus/client_golang/prometheus"
	"github.com/prometheus/client_golang/prometheus/promhttp"
)

const (
	// default port for the HTTP server
	defaultPort = "9090"
)

// healthResponse represents the JSON structure returned by /health.
type healthResponse struct {
	Status       string        `json:"status"`
	NodeEndpoint string        `json:"node_endpoint"`
	BlockHeight  uint64        `json:"block_height"`
	ChainID      uint64        `json:"chain_id"`
	Syncing      bool          `json:"syncing"`
	SyncProgress *syncProgress `json:"sync_progress,omitempty"`
	Timestamp    string        `json:"timestamp"`
	Error        string        `json:"error,omitempty"`
}

// syncProgress provides sync details when the node is syncing.
type syncProgress struct {
	StartingBlock uint64 `json:"starting_block"`
	CurrentBlock  uint64 `json:"current_block"`
	HighestBlock  uint64 `json:"highest_block"`
}

func main() {
	// read required environment variable
	rpcEndpoint := os.Getenv("GOAT_RPC_NODE")
	if rpcEndpoint == "" {
		log.Fatal("GOAT_RPC_NODE environment variable is required")
	}

	port := os.Getenv("PORT")
	if port == "" {
		port = defaultPort
	}

	log.Printf("starting goat-monitor on :%s", port)
	log.Printf("monitoring RPC endpoint: %s", rpcEndpoint)

	// initialize RPC client
	client := rpc.NewClient(rpcEndpoint)

	// register Prometheus collector
	goatCollector := collector.NewGoatCollector(client)
	prometheus.MustRegister(goatCollector)

	// HTTP routes
	mux := http.NewServeMux()

	// prometheus metrics endpoint
	mux.Handle("/metrics", promhttp.Handler())

	// JSON health dashboard
	mux.HandleFunc("/health", func(w http.ResponseWriter, r *http.Request) {
		healthHandler(w, r, client, rpcEndpoint)
	})

	// root redirects to /health
	mux.HandleFunc("/", func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/" {
			http.NotFound(w, r)
			return
		}
		http.Redirect(w, r, "/health", http.StatusTemporaryRedirect)
	})

	// start server
	server := &http.Server{
		Addr:         fmt.Sprintf(":%s", port),
		Handler:      mux,
		ReadTimeout:  15 * time.Second,
		WriteTimeout: 15 * time.Second,
		IdleTimeout:  60 * time.Second,
	}

	if err := server.ListenAndServe(); err != nil {
		log.Fatalf("server failed: %v", err)
	}
}

// healthHandler queries the RPC node and returns a JSON health response.
func healthHandler(w http.ResponseWriter, _ *http.Request, client *rpc.Client, endpoint string) {
	resp := healthResponse{
		Status:       "ok",
		NodeEndpoint: endpoint,
		Timestamp:    time.Now().UTC().Format(time.RFC3339),
	}

	// fetch block height
	block, err := client.GetBlockNumber()
	if err != nil {
		resp.Status = "degraded"
		resp.Error = fmt.Sprintf("block number: %v", err)
	}
	resp.BlockHeight = block

	// fetch chain ID
	chainID, err := client.GetChainID()
	if err != nil {
		resp.Status = "degraded"
		if resp.Error != "" {
			resp.Error += "; "
		}
		resp.Error += fmt.Sprintf("chain id: %v", err)
	}
	resp.ChainID = chainID

	// fetch sync status
	syncing, progress, err := client.GetSyncStatus()
	if err != nil {
		resp.Status = "degraded"
		if resp.Error != "" {
			resp.Error += "; "
		}
		resp.Error += fmt.Sprintf("sync status: %v", err)
	}
	resp.Syncing = syncing

	if progress != nil {
		resp.SyncProgress = &syncProgress{
			StartingBlock: progress.StartingBlock,
			CurrentBlock:  progress.CurrentBlock,
			HighestBlock:  progress.HighestBlock,
		}
	}

	w.Header().Set("Content-Type", "application/json")
	if resp.Status != "ok" {
		w.WriteHeader(http.StatusServiceUnavailable)
	}

	enc := json.NewEncoder(w)
	enc.SetIndent("", "  ")
	if err := enc.Encode(resp); err != nil {
		log.Printf("error encoding health response: %v", err)
	}
}

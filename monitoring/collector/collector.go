// package collector implements a Prometheus collector that queries
// a goat (EVM-compatible) RPC node for block height, chain ID, and sync status.
package collector

import (
	"log"

	"github.com/layerzero-sre/goat-monitor/rpc"
	"github.com/prometheus/client_golang/prometheus"
)

const namespace = "goat"

// GoatCollector collects metrics from a goat RPC node.
type GoatCollector struct {
	client *rpc.Client

	// metric descriptors
	blockHeight *prometheus.Desc
	chainID     *prometheus.Desc
	syncing     *prometheus.Desc
	rpcUp       *prometheus.Desc
}

// NewGoatCollector creates a new collector for the given RPC client.
func NewGoatCollector(client *rpc.Client) *GoatCollector {
	return &GoatCollector{
		client: client,
		blockHeight: prometheus.NewDesc(
			prometheus.BuildFQName(namespace, "", "block_height"),
			"current block height of the goat node",
			nil, nil,
		),
		chainID: prometheus.NewDesc(
			prometheus.BuildFQName(namespace, "", "chain_id"),
			"chain ID reported by the goat node",
			nil, nil,
		),
		syncing: prometheus.NewDesc(
			prometheus.BuildFQName(namespace, "", "syncing"),
			"whether the goat node is syncing (1=syncing, 0=synced)",
			nil, nil,
		),
		rpcUp: prometheus.NewDesc(
			prometheus.BuildFQName(namespace, "", "rpc_up"),
			"whether the goat RPC endpoint is reachable (1=up, 0=down)",
			nil, nil,
		),
	}
}

// Describe sends the descriptor for each metric to the provided channel.
func (c *GoatCollector) Describe(ch chan<- *prometheus.Desc) {
	ch <- c.blockHeight
	ch <- c.chainID
	ch <- c.syncing
	ch <- c.rpcUp
}

// Collect queries the RPC node and sends metric values to the provided channel.
func (c *GoatCollector) Collect(ch chan<- prometheus.Metric) {
	up := 1.0

	// fetch block height
	block, err := c.client.GetBlockNumber()
	if err != nil {
		log.Printf("error fetching block number: %v", err)
		up = 0.0
	}
	ch <- prometheus.MustNewConstMetric(c.blockHeight, prometheus.GaugeValue, float64(block))

	// fetch chain ID
	chain, err := c.client.GetChainID()
	if err != nil {
		log.Printf("error fetching chain id: %v", err)
		up = 0.0
	}
	ch <- prometheus.MustNewConstMetric(c.chainID, prometheus.GaugeValue, float64(chain))

	// fetch sync status
	isSyncing, _, err := c.client.GetSyncStatus()
	if err != nil {
		log.Printf("error fetching sync status: %v", err)
		up = 0.0
	}
	syncVal := 0.0
	if isSyncing {
		syncVal = 1.0
	}
	ch <- prometheus.MustNewConstMetric(c.syncing, prometheus.GaugeValue, syncVal)

	// report RPC availability
	ch <- prometheus.MustNewConstMetric(c.rpcUp, prometheus.GaugeValue, up)
}

# SRE Challenge

A production-grade Goat RPC node deployment with a custom Prometheus monitoring exporter, containerized with Docker and orchestrated via Kubernetes.

## Architecture

```
┌─────────────────────────────────────────────────────────────────┐
│                        Docker Compose / K8s                      │
│                                                                  │
│  ┌──────────────────────────────────┐  ┌──────────────────────┐  │
│  │         Goat RPC Node            │  │   Monitoring Exporter │  │
│  │                                  │  │                       │  │
│  │  ┌─────────┐    ┌────────────┐   │  │  ┌───────────────┐   │  │
│  │  │  geth   │    │    goat    │   │  │  │ goat-monitor  │   │  │
│  │  │ (EVM)   │    │(consensus)│   │  │  │               │   │  │
│  │  │ :8545   │◄───│  :26657   │   │  │  │  :9090        │   │  │
│  │  │ :8546   │    │  :26656   │   │  │  │  /metrics     │   │  │
│  │  └─────────┘    └────────────┘   │  │  │  /health      │   │  │
│  │                                  │  │  └───────┬───────┘   │  │
│  └──────────────────────────────────┘  └──────────┼───────────┘  │
│                  ▲                                │               │
│                  │         JSON-RPC (eth_*)       │               │
│                  └───────────────────────────────-┘               │
└─────────────────────────────────────────────────────────────────┘
```

## Monitored Metrics

| Metric | Prometheus Name | RPC Method | Description |
|--------|----------------|------------|-------------|
| Block Height | `goat_block_height` | `eth_blockNumber` | current block number |
| Chain ID | `goat_chain_id` | `eth_chainId` | network identifier (expected: `2345`) |
| Sync Status | `goat_syncing` | `eth_syncing` | `1` = syncing, `0` = synced |
| RPC Status | `goat_rpc_up` | all | `1` = reachable, `0` = unreachable |

## Prerequisites

- [Docker](https://docs.docker.com/get-docker/) & [Docker Compose](https://docs.docker.com/compose/install/)
- [kubectl](https://kubernetes.io/docs/tasks/tools/)
- [k3d](https://k3d.io/) (for Part 2 — Kubernetes deployment)

---

## Part 1: Building and Running

Docker Compose uses [profiles](https://docs.docker.com/compose/profiles/) to separate the monitoring exporter (runs by default) from the full Goat node (opt-in).

### Option A: Monitor Only (Public RPC)

Run just the monitoring exporter against a public Goat RPC endpoint — no local node required:

```bash
cd ${repo_dir}

# set the RPC to the public endpoint
echo "GOAT_RPC_NODE=https://rpc.goat.network" > .env

# build and start the monitoring exporter
docker compose up -d --build

# verify
docker compose ps
curl http://localhost:9090/health
curl http://localhost:9090/metrics
```

### Option B: Full Stack (Local Node + Monitoring)

Run the Goat RPC node (geth + goat) alongside the monitoring exporter:

```bash
cd ${repo_dir}

# use the default .env which points at the local geth node
cp .env.example .env

# start all services — geth, goat consensus, and monitor
docker compose --profile node up -d --build

# verify all containers
docker compose --profile node ps
curl http://localhost:9090/health
```

> **Note:** the GHCR images are public, but stale Docker credentials can cause
> `denied` errors. Fix with: `docker logout ghcr.io`

> **Note:** on first start, geth will begin syncing from block 0. The goat
> consensus container may restart until the execution layer catches up — this is
> normal behavior.

### Standalone Docker (No Compose)

Build and run the monitoring image directly:

```bash
cd ${repo_dir}/monitoring
docker build -t george-goat-monitor:latest .

# run against any RPC endpoint
docker run -d \
  --name george-goat-monitor \
  -p 9090:9090 \
  -e GOAT_RPC_NODE=https://rpc.goat.network \
  george-goat-monitor:latest

# verify
curl http://localhost:9090/health
curl http://localhost:9090/metrics
```

### Expected Output

**`/health` endpoint** returns JSON:

```json
{
  "status": "ok",
  "node_endpoint": "https://rpc.goat.network",
  "block_height": 10235456,
  "chain_id": 2345,
  "syncing": false,
  "timestamp": "2026-02-17T00:33:50Z"
}
```

**`/metrics` endpoint** returns Prometheus format:

```
# HELP goat_block_height current block height of the goat node
# TYPE goat_block_height gauge
goat_block_height 1.0235456e+07
# HELP goat_chain_id chain ID reported by the goat node
# TYPE goat_chain_id gauge
goat_chain_id 2345
# HELP goat_syncing whether the goat node is syncing (1=syncing, 0=synced)
# TYPE goat_syncing gauge
goat_syncing 0
# HELP goat_rpc_up whether the goat RPC endpoint is reachable (1=up, 0=down)
# TYPE goat_rpc_up gauge
goat_rpc_up 1
```

### Cleanup

```bash
# monitor only
docker compose down

# full stack
docker compose --profile node down

# standalone
docker stop george-goat-monitor && docker rm george-goat-monitor
```

---

## Part 2: Kubernetes Deployment

### Manifests Overview

```
k8s/
├── namespace.yaml              # goat-network namespace
├── kustomization.yaml          # apply everything at once
├── goat-node/
│   ├── deployment.yaml         # geth + goat sidecar containers with PVCs
│   ├── service.yaml            # ClusterIP service (ports 8545, 8546)
│   └── ingress.yaml            # ingress at goat-rpc.local
└── monitoring/
    ├── secret.yaml             # GOAT_RPC_NODE as K8s Secret
    ├── deployment.yaml         # monitoring exporter pod
    ├── service.yaml            # ClusterIP service (port 9090)
    └── ingress.yaml            # ingress at goat-monitor.local
```

### Deploy with k3d

k3d includes Traefik as its default ingress controller — no additional setup required.

```bash
# 1. create a k3d cluster with port mappings for ingress
k3d cluster create goat-sre \
  --port "8080:80@loadbalancer" \
  --port "8443:443@loadbalancer" \
  --agents 1

# 2. build and import the monitoring image
cd ${repo_dir}/monitoring
docker build -t goat-monitor:latest .
k3d image import goat-monitor:latest -c goat-sre
cd ..

# 3. apply all Kubernetes manifests
kubectl apply -k k8s/

# 4. verify pods are running
kubectl get pods -n goat-network

# 5. access the services via Traefik ingress (port 8080)
curl -H "Host: goat-monitor.local" http://localhost:8080/health
curl -H "Host: goat-monitor.local" http://localhost:8080/metrics
```

> **Tip:** alternatively, add DNS entries to `/etc/hosts` and access directly:
> ```bash
> echo "127.0.0.1 goat-rpc.local goat-monitor.local" | sudo tee -a /etc/hosts
> curl http://goat-monitor.local:8080/health
> ```

### Customizing the Secret

To point the monitoring exporter at a different RPC endpoint:

```bash
# encode your endpoint
echo -n "https://rpc.goat.network" | base64
# output: aHR0cHM6Ly9ycGMuZ29hdC5uZXR3b3Jr

# update k8s/monitoring/secret.yaml with the new base64 value
# then re-apply
kubectl apply -f k8s/monitoring/secret.yaml
kubectl rollout restart deployment/goat-monitor -n goat-network
```

### Cleanup

```bash
# delete all resources
kubectl delete -k k8s/

# delete the k3d cluster
k3d cluster delete goat-sre
```

### Deploying to Other Kubernetes Environments

The manifests are tested with k3d (Traefik) but work on any Kubernetes distribution with minor adjustments:

**Ingress controller** — the ingress manifests omit `ingressClassName` since k3d's Traefik auto-claims them. For other clusters, add the appropriate class to both `k8s/goat-node/ingress.yaml` and `k8s/monitoring/ingress.yaml`:

```yaml
spec:
  ingressClassName: nginx  # or traefik, alb, etc.
```

**PVC sizing** — the PVCs are set to 5Gi/1Gi for local testing. For production nodes, increase `geth-data-pvc` to at least 100Gi in `k8s/goat-node/deployment.yaml`.

**Monitor image** — instead of `k3d image import`, push the image to a container registry your cluster can pull from:

```bash
docker tag goat-monitor:latest <your-registry>/goat-monitor:latest
docker push <your-registry>/goat-monitor:latest
```

Then update the image reference in `k8s/monitoring/deployment.yaml`.

---

## Configuration Reference

| Variable | Default | Description |
|----------|---------|-------------|
| `GOAT_RPC_NODE` | `http://geth:8545` | goat RPC endpoint URL. set to `https://rpc.goat.network` for monitor-only mode |
| `PORT` | `9090` | HTTP server port for the monitoring exporter |

## Project Structure

```
layerzero-sre-challenge/
├── README.md                   # this file
├── .env.example                # environment template
├── .gitignore
├── docker-compose.yml          # full-stack: geth + goat + monitor
│
├── monitoring/                 # Go monitoring exporter
│   ├── Dockerfile              # multi-stage build
│   ├── go.mod / go.sum
│   ├── main.go                 # HTTP server entry point
│   ├── collector/
│   │   └── collector.go        # Prometheus collector
│   └── rpc/
│       └── client.go           # JSON-RPC client
│
└── k8s/                        # Kubernetes manifests
    ├── namespace.yaml
    ├── kustomization.yaml
    ├── goat-node/              # RPC node deployment + service + ingress
    └── monitoring/             # monitor deployment + service + ingress + secret
```

## Design Decisions

1. **Go for the exporter**: chosen because it produces minimal Docker images (~15MB), and Go is the standard language for Prometheus exporters.

2. **Multi-stage Docker build**: separates build dependencies from runtime, resulting in a small, secure image with only the compiled binary and CA certificates.

3. **Non-root container**: the monitoring image runs as a dedicated `monitor` user (UID 1000) for security.

4. **Sidecar pattern in K8s**: geth and goat run as sidecar containers in the same pod since they're tightly coupled (shared lifecycle, shared data path).

5. **Kustomize over Helm**: simpler approach that meets the challenge requirements without adding Helm templating complexity.

6. **Health probes**: both the Goat node and monitoring exporter have liveness and readiness probes for proper K8s lifecycle management.

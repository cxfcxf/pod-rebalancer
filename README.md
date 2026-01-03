# Pod Rebalancer

A Kubernetes operator that automatically redistributes pods across nodes to maintain balance. Built with [kubebuilder](https://kubebuilder.io/).

## Features

- **Automatic rebalancing** - Triggers when nodes are added or removed from the cluster
- **Interval-based rebalancing** - Continuously maintains balance at configurable intervals
- **Hardware-specific targets** - Define different pod counts for different node types
- **Rolling eviction** - Evicts pods in batches with configurable delays
- **PDB-aware** - Uses the Kubernetes eviction API to respect PodDisruptionBudgets
- **Dry-run mode** - Preview what would be evicted without making changes

## Installation

```bash
# Install CRDs
make install

# Deploy the operator
make deploy IMG=ghcr.io/cxfcxf/pod-rebalancer:latest
```

## Usage

### 1. Label pods for rebalancing

Only pods with the `kore.boring.io/rebalance: "true"` label are considered for rebalancing:

```bash
kubectl label pod <pod-name> kore.boring.io/rebalance=true
```

Or add it to your Deployment/StatefulSet template:

```yaml
spec:
  template:
    metadata:
      labels:
        kore.boring.io/rebalance: "true"
```

### 2. Create a RebalanceRequest

#### One-shot rebalance

```yaml
apiVersion: kore.boring.io/v1alpha1
kind: RebalanceRequest
metadata:
  name: manual-rebalance
spec:
  batchSize: 5
  batchIntervalSeconds: 30
  dryRun: false
```

#### Continuous rebalancing with hardware-specific targets

```yaml
apiVersion: kore.boring.io/v1alpha1
kind: RebalanceRequest
metadata:
  name: continuous-rebalance
spec:
  # Run every 5 minutes
  intervalSeconds: 300

  # Define target pods per node type
  nodeTargets:
    - nodeSelector:
        hardware: high-memory
      targetPodsPerNode: 7
    - nodeSelector:
        hardware: standard
      targetPodsPerNode: 5

  batchSize: 3
  batchIntervalSeconds: 15
```

### 3. Monitor status

```bash
kubectl get rebalancerequests

NAME                   PHASE     INTERVAL   RUNS   EVICTED   LASTRUN   AGE
continuous-rebalance   Active    300        12     24        2m        1h
manual-rebalance       Completed            1      8                   5m
```

## Configuration

### RebalanceRequest Spec

| Field | Type | Description |
|-------|------|-------------|
| `intervalSeconds` | int32 | How often to run (0 = one-shot) |
| `nodeTargets` | []NodeTarget | Per-node-type pod targets |
| `selector` | LabelSelector | Additional pod label filter |
| `namespaces` | []string | Target namespaces (empty = all) |
| `batchSize` | int32 | Pods to evict per batch (default: 5) |
| `batchIntervalSeconds` | int32 | Delay between batches (default: 30) |
| `dryRun` | bool | Preview mode without evictions |

### NodeTarget

| Field | Type | Description |
|-------|------|-------------|
| `nodeSelector` | map[string]string | Node label selector |
| `targetPodsPerNode` | int32 | Desired pod count per matching node |

## How it works

1. **Node Controller** watches for node additions/removals and auto-creates RebalanceRequests
2. **Rebalance Controller** processes RebalanceRequests:
   - Finds candidate pods (labeled with `kore.boring.io/rebalance: "true"`)
   - Calculates target distribution based on `nodeTargets` or even distribution
   - Identifies nodes with excess pods
   - Evicts pods in rolling batches using the eviction API
3. For interval-based requests, the controller reschedules itself after each run

## Development

```bash
# Run locally
make run

# Run tests
make test

# Build container
make docker-build IMG=<your-registry>/pod-rebalancer:latest

# Generate manifests after CRD changes
make manifests generate
```

## License

Apache 2.0

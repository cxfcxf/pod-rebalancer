# Pod Rebalancer

A Kubernetes operator that continuously maintains pod distribution across nodes. Built with [kubebuilder](https://kubebuilder.io/).

## Features

- **Continuous rebalancing** - Runs at configurable intervals to maintain balance
- **Hardware-specific limits** - Define maximum pod counts per node type (e.g., 7 pods on high-memory nodes, 5 on standard)
- **Maximum enforcement** - Only evicts pods when nodes exceed their limit, allowing temporary overflow during node failures
- **Rolling eviction** - Evicts pods in batches with configurable delays
- **PDB-aware** - Uses the Kubernetes eviction API to respect PodDisruptionBudgets
- **Dry-run mode** - Preview what would be evicted without making changes

## How it works

1. The operator runs at a configurable interval (default: 60 seconds)
2. For each check, it finds pods with the `kore.boring.io/rebalance: "true"` label
3. It counts pods per node and compares against configured maximums
4. Pods on nodes exceeding their maximum are evicted (newest first)
5. Evicted pods are rescheduled by their controllers to nodes with capacity

**Key behavior**: Maximums are soft limits. When a node fails, remaining nodes can temporarily exceed their max. The rebalancer only evicts once a new node joins and has capacity.

## Installation

```bash
# Install CRDs
make install

# Deploy the operator
make deploy IMG=ghcr.io/cxfcxf/pod-rebalancer:latest
```

## Usage

### 1. Label pods for rebalancing

Only pods with the `kore.boring.io/rebalance: "true"` label are managed:

```yaml
spec:
  template:
    metadata:
      labels:
        kore.boring.io/rebalance: "true"
```

### 2. Create a RebalanceRequest

```yaml
apiVersion: kore.boring.io/v1alpha1
kind: RebalanceRequest
metadata:
  name: pod-rebalancer
spec:
  # Check every 60 seconds
  intervalSeconds: 60

  # Define max pods per hardware type
  nodeTargets:
    - nodeSelector:
        hardware: high-memory
      maxPodsPerNode: 7
    - nodeSelector:
        hardware: standard
      maxPodsPerNode: 5

  batchSize: 3
  batchIntervalSeconds: 15
```

### 3. Monitor status

```bash
kubectl get rebalancerequests

NAME             PHASE    INTERVAL   RUNS   EVICTED   LASTRUN   AGE
pod-rebalancer   Active   60         42     12        30s       1h
```

## Configuration

### RebalanceRequest Spec

| Field | Type | Default | Description |
|-------|------|---------|-------------|
| `intervalSeconds` | int32 | 60 | How often to check balance (min: 30) |
| `nodeTargets` | []NodeTarget | - | Per-node-type maximum pod counts |
| `selector` | LabelSelector | - | Additional pod label filter |
| `namespaces` | []string | all | Target namespaces |
| `batchSize` | int32 | 5 | Pods to evict per batch |
| `batchIntervalSeconds` | int32 | 30 | Delay between batches |
| `dryRun` | bool | false | Preview mode |

### NodeTarget

| Field | Type | Description |
|-------|------|-------------|
| `nodeSelector` | map[string]string | Node label selector |
| `maxPodsPerNode` | int32 | Maximum pods allowed on matching nodes |

## Example scenarios

### Node failure

- 3 nodes × 7 max pods = 21 pods running
- Node fails → 14 running + 7 rescheduled
- 7 pods land on remaining 2 nodes (now at 10-11 each, exceeding max)
- **Rebalancer does nothing** - no node has capacity
- New node joins → rebalancer evicts excess pods → they schedule to new node

### New node added

- 2 nodes with 10 pods each (max is 7)
- New node joins with 0 pods
- Rebalancer evicts 3 pods from each overloaded node
- Pods reschedule to the new node

## Development

```bash
# Run locally
make run

# Build container
make docker-build IMG=<your-registry>/pod-rebalancer:latest

# Generate manifests after CRD changes
make manifests generate
```

## License

Apache 2.0

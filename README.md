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
3. It calculates proportional targets based on each node's capacity
4. Pods on nodes exceeding their target are evicted (newest first)
5. Evicted pods are rescheduled by their controllers to nodes with capacity

**Key behavior**: The rebalancer uses capacity-proportional distribution. When a new node joins, existing pods are rebalanced to utilize the new capacity, even if no node was "overloaded".

## Algorithm

The rebalancer uses a **capacity-proportional** algorithm to distribute pods across nodes based on their configured maximum capacity.

### Calculation

For each node, the target pod count is calculated as:

```
target = (nodeMaxPods / totalClusterCapacity) × totalPods + 1
```

Where:
- `nodeMaxPods` = configured max for this node type
- `totalClusterCapacity` = sum of all nodes' max pods
- `totalPods` = current total pods being managed
- `+1` = slack to avoid constant evictions

Pods are evicted from any node where `currentPods > target`.

### Scenario: Adding a new node (heterogeneous cluster)

```
Before:
  Node A (high-memory): 15 pods, max 15
  Node B (standard):     8 pods, max 8

After adding Node C (high-memory, max 15):
  Total pods: 23
  Total capacity: 15 + 8 + 15 = 38

  Target calculation:
    Node A: (15/38) × 23 + 1 = 10
    Node B: (8/38)  × 23 + 1 = 5
    Node C: (15/38) × 23 + 1 = 10

  Evictions:
    Node A: 15 - 10 = 5 pods evicted
    Node B: 8 - 5   = 3 pods evicted
    Node C: 0 pods (receives evicted pods)

  Result: A=10, B=5, C=8 (proportionally balanced)
```

### Scenario: Homogeneous cluster (even spread)

When all nodes have the same `maxPodsPerNode`, the algorithm produces an even spread:

```
3 nodes, all max 10, total 15 pods
Total capacity: 30

Each node target: (10/30) × 15 + 1 = 6

If distribution is A=10, B=3, C=2:
  Node A: 10 - 6 = 4 pods evicted
  Node B: 3 - 6 = ok
  Node C: 2 - 6 = ok

Result: A=6, B=5, C=4 (evenly spread)
```

### Scenario: Node failure (graceful handling)

```
Before:
  3 nodes × 7 max pods = 21 pods running

Node fails:
  2 remaining nodes now have ~10-11 pods each
  Rebalancer calculates: no capacity available (total capacity = 14, total pods = 21)
  Result: NO evictions (nowhere to put them)

New node joins:
  Total capacity restored to 21
  Rebalancer evicts excess pods to new node
```

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

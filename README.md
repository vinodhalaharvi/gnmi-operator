# gnmi-operator

A Kubernetes operator that expresses network intent as custom resources and
reconciles it against a device over
[gNMI](https://openconfig.net/docs/gnmi/gnmi-specification/).

This is a learning project. The goal is not a production multi-vendor
abstraction — it is to discover first-hand why mature network operators end up
needing reconciliation, status conditions, finalizers, drift detection, work
queues and connection management.

```
Kubernetes API
     |  watches custom resources
     v
InterfaceReconciler / DeviceReconciler
     |  gNMI Get / Set
     v
gNMI-capable target
```

## Status

| Piece | State |
| --- | --- |
| `Interface` and `Device` CRDs | Done — validating and defaulting in-cluster |
| `internal/gnmi` client | Done — Capabilities, Get, Set, TypedValue decoding |
| `cmd/gnmiprobe` | Done — standalone Capabilities → Get → Set → verify |
| Reconcilers | In progress |
| Drift detection, Subscribe, finalizers | Not started |

Not usable as an operator yet. The CRDs are real and the gNMI client works;
the control loop is what remains.

## Design principle

Do not reimplement gNMI; implement our own understanding of how a controller
should *use* it. We depend on `google.golang.org/grpc` and the official
`github.com/openconfig/gnmi/proto/gnmi` messages, and deliberately construct
real `Path`, `GetRequest`, `SetRequest` and `TypedValue` objects rather than
hiding them behind convenience helpers.

## API

```yaml
apiVersion: network.lab/v1alpha1
kind: Device
metadata:
  name: linux1
spec:
  address: 127.0.0.1
  insecure: true          # lab only; omit and set tlsSecretRef in practice
---
apiVersion: network.lab/v1alpha1
kind: Interface
metadata:
  name: linux1-eth1
spec:
  deviceRef:
    name: linux1
  name: eth1              # the interface name on the device
  enabled: true
  mtu: 1500
```

`enabled` and `mtu` are pointers in Go. Omitting one means "do not manage this
leaf" — distinct from setting it to a zero value. Without that distinction
every Interface would silently assert `enabled: false` on creation.

`spec.port` defaults to 9339. `deviceRef.name` is enforced non-empty by a CEL
rule, because `corev1.LocalObjectReference` allows an empty name for
backwards-compatibility reasons that don't apply here.

## Lab environment

Developed on Apple Silicon against a single Ubuntu 24.04 ARM64 VM (Lima, `vz`
VM type) hosting a kubeadm cluster, Docker and containerlab. Keeping the
cluster and the devices on one Linux kernel means the operator reaches a device
over a plain route and the gNMI session can be captured with `tcpdump`.

Pod CIDR is `10.244.0.0/16` with Calico; containerlab defaults to
`172.20.0.0/16`. Keep custom topologies out of both.

### Targets

| Target | What is real |
| --- | --- |
| `google/gnxi` `gnmi_target` | Protocol is real; the config tree is in memory |
| Nokia SR Linux via containerlab | A real NOS with real commit semantics |

No gNMI Set in this lab programs the host's own netlink state. gNMI lives
inside a vendor NOS, where one process owns both the config tree and the
forwarding state; on a plain Linux box nothing fills that role. This is a
property of the protocol's deployment model, not a limitation of the
controller.

Note that gnxi's target serves no `state` subtree — it stores what you seed and
synthesizes nothing. Reads fall back to `config`, which is why the client
distinguishes `NotFound` (the leaf is absent) from a dial failure (the device
is unreachable).

## Running the lab

```bash
go install github.com/google/gnxi/gnmi_target@latest
gnmi_target -bind_address :9339 -config lab/target-config.json -notls
```

In another shell:

```bash
go run ./cmd/gnmiprobe -insecure -target 127.0.0.1:9339 -interface eth1 -mtu 9000
```

Run it twice. The second run reports that desired matches observed and issues
no Set — the idempotency the reconciler has to inherit.

```bash
make manifests generate
kubectl apply -f config/crd/bases/
make run
```

Running the manager on the lab host rather than deploying it into the cluster
keeps the early phases free of pod networking concerns.

## Layout

```
api/v1alpha1/          Interface and Device types
internal/gnmi/         reusable client and path builders
internal/controller/   reconcilers
cmd/gnmiprobe/         standalone protocol exercise
cmd/main.go            manager entrypoint
lab/                   target seed config
```

`internal/gnmi` never calls `log.Fatal` or `os.Exit` and registers no flags —
it is library code called from a reconcile loop, so every failure returns an
error.

## Roadmap

1. Connect to a target; Capabilities and Get — done
2. A controlled Set — done
3. The Interface and Device CRDs — done
4. Reconcile desired state to gNMI state — in progress
5. `.status` and Kubernetes conditions
6. Periodic drift detection
7. `Subscribe` for event-driven reconciliation
8. Connection lifecycle, retry/backoff, TLS and credentials
9. Linux + FRR routing use cases
10. A real NOS (SR Linux, cEOS)
11. Juniper/Cisco and model portability
12. Richer intent: BGP, VRFs, VLANs

## Problems this project exists to hit

Drift when something changes the device outside Kubernetes. Idempotency.
Representing unreachable devices in status. Whether deleting a CR should
unconfigure the device. Two resources claiming the same interface. Atomicity
across multiple gNMI paths — a single `SetRequest` is atomic, several are not.
Connection scale. And how portable "OpenConfig" intent really is between
implementations.

## References

- [gNMI specification](https://openconfig.net/docs/gnmi/gnmi-specification/)
- [openconfig/gnmi](https://github.com/openconfig/gnmi)
- [google/gnxi](https://github.com/google/gnxi)
- [Kubebuilder](https://kubebuilder.io/)
- [Containerlab](https://containerlab.dev/)

## License

MIT



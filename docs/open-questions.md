# Open Questions

Single remaining unresolved decision, carried from [plan.md](../plan.md) §8:

## Sandbox runtime for preview/deploy

Options considered:
1. **Freestyle VMs** (freestyle.sh) — sandbox-as-a-service, handles VM
   provisioning, snapshots, networking, domains. Least infra to build.
2. **Self-managed containers** (Docker/Firecracker) — full control, no
   third-party dependency, significantly more infra work.
3. Left undecided intentionally — the build order ([plan.md](../plan.md) §9)
   stubs preview behind an interface so the pipeline can be built and tested
   end-to-end before this is resolved.

**Interface contract to keep the decision swappable**: implement
`internal/sandbox.Provisioner` with:
```go
type Provisioner interface {
    Provision(ctx context.Context, runID uuid.UUID, files map[string]string) (previewURL string, expiresAt time.Time, err error)
    Promote(ctx context.Context, appID uuid.UUID, runID uuid.UUID) (liveURL string, err error)
    Teardown(ctx context.Context, runID uuid.UUID) error
}
```
Build order step 8 ("Preview") picks the concrete implementation
(`FreestyleProvisioner` or `ContainerProvisioner`) behind this interface —
everything upstream (workflow, API, frontend) only depends on the interface,
so resolving this later doesn't require touching the pipeline.

Resolve this before build order step 8; not required before steps 1–7.

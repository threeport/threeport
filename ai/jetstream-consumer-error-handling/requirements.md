# Requirements: fix silent JetStream consumer failure in Threeport controllers

## Background

Threeport is a control-plane framework. Each object type (organizations, users,
roles, control planes, secrets, etc.) has a generated controller process that
reconciles changes by listening for events on a NATS JetStream durable
consumer. All of these controller `main.go` files are **generated code**,
produced by a shared code generator in `threeport/0.7`
(`pkg/sdk/v0/gen/cmd/controller/main.go`). Both base Threeport's own
controllers (13 of them) and the `qleet-threeport-module` extension's
controllers (5 of them: organizations, users, roles, policies, control-planes)
are produced by this same generator.

## Problem statement (incident that motivated this work)

On a remote dev environment ("dev-18"), users who registered through a web
portal got their organization created in the database, but the asynchronous
step that provisions their API gateway route never ran. The organization
looked "stuck" — no error was ever surfaced anywhere, the controller pod
reported healthy to Kubernetes the entire time, and this persisted for over
four days across a redeploy.

Root cause: the controller's one-time startup call to create its JetStream
durable consumer (`js.AddConsumer(...)`) failed once (most likely a transient
timing issue during a coordinated redeploy where the NATS server came up
*after* the controller did), and **the code does not check the error returned
by that call**. The controller process kept running, `/readyz` kept returning
a static `200 OK`, and the reconcile loop's message-fetch errors were logged
but treated as routine/non-fatal — so nothing in the system was ever able to
detect or recover from the failure. A plain pod restart (once NATS was
confirmed healthy) immediately fixed it, which confirms the defect is purely
in error handling/observability, not in the reconciliation logic itself.

This was reproduced identically in a sibling case: base Threeport's own
`threeport-api-server` silently failed to (re)create its NATS streams after a
similar redeploy race, and multiple base controllers (`control-plane`,
`secret`, `gcp`, `aws`, `oci`, `kubernetes-runtime`, `kubernetes-workload`,
`gateway`, `observability`, `terraform`) were left with no working consumers
for days, with zero indication in their health status.

**This document is scoped to fixing the code so this class of failure becomes
loud (fails fast, restarts, recovers) instead of silent. The live dev-18
environment has already been worked around operationally (pod restarts) —
that is done and is not part of this task.**

## Root cause detail

File: `threeport/0.7/pkg/sdk/v0/gen/cmd/controller/main.go`

Function: `ConfigurePullSubscription` (defines the generated code that runs at
controller startup, inside the per-reconciler setup loop in
`GenControllerMain`).

Current generated code (~line 479, inside the `if durable` branch):

```go
g.Id("js").Dot("AddConsumer").Call(Qual(
    fmt.Sprintf("%s/internal/%s/notif", modulePath, objGroup.ControllerShortName),
    objGroup.StreamName,
).Op(",").Op("&").Qual(
    "github.com/nats-io/nats.go", "ConsumerConfig",
).Values(Dict{
    Id("Durable"):      consumer,
    Id("AckPolicy"):    Qual("github.com/nats-io/nats.go", "AckExplicitPolicy"),
    Id("FilterSubject"): Id("r").Dot("NotifSubject"),
}),
)
```

This emits `js.AddConsumer(stream, &nats.ConsumerConfig{...})` as a bare
statement in the generated `main_gen.go` — **both return values (the created
consumer info and the error) are discarded.** If this call fails, the
controller has no consumer, silently, forever (nothing else re-attempts it).

Immediately below this in the same function (~line 501-524), the sibling call
to `js.PullSubscribe(...)` **does** check its error correctly:

```go
g.Id("sub").Op(",").Id("err").Op(":=").Id("js").Dot("PullSubscribe").Call(...)
g.If(Id("err").Op("!=").Nil()).Block(
    Id("log").Dot("Error").Call(Id("err"), Lit("failed to create pull subscription for reconciler notifications"), Lit("reconcilerName"), Id("r").Dot("Name")),
    Qual("os", "Exit").Call(Lit(1)),
)
```

The `AddConsumer` call should follow the exact same pattern. This is clearly
an oversight/inconsistency within the same function, not an intentional
design choice — the very next lines already establish the desired
error-handling convention for this controller.

Two more locations compound the silence once a consumer is missing:

1. **`/readyz` is a hardcoded 200**, unconditionally, regardless of whether
   the controller's subscription is actually alive. This is generated in
   `GenControllerMain` in the same file (~line 404-415):
   ```go
   Id("http.HandleFunc").Call(
       Lit("/readyz"),
       Func().Params(Id("w").Qual("net/http", "ResponseWriter"), Id("r").Op("*").Qual("net/http", "Request")).Block(
           Id("w").Dot("WriteHeader").Call(Qual("net/http", "StatusOK")),
           Id("w").Dot("Write").Call(Index().Byte().Call(Lit("OK"))),
       ),
   )
   ```
   Kubernetes readiness/liveness probes hitting this endpoint have no way to
   detect a controller that is running but not actually listening.

2. **The reconcile loop treats a structurally broken consumer as routine.**
   File: `threeport/0.7/pkg/controller/v0/reconcile.go`, function
   `PullMessage` (~line 91-107). Any `Sub.Fetch` error other than a bare
   `nats.ErrTimeout` is logged and the function returns `nil` — the loop just
   keeps polling forever. A permanent error like "consumer not found" gets
   logged repeatedly but never escalated.

## Required changes

### Part 1 — fix the generator (`threeport/0.7`)

1. In `pkg/sdk/v0/gen/cmd/controller/main.go`, `ConfigurePullSubscription`:
   capture the error from the `AddConsumer` call and handle it the same way
   the adjacent `PullSubscribe` error is handled — log with
   `log.Error(err, "failed to create JetStream consumer for reconciler
   notifications", "reconcilerName", r.Name)` and `os.Exit(1)`. Startup-time
   `os.Exit(1)` is appropriate here (matches the existing sibling pattern) so
   Kubernetes restarts the pod and retries, rather than running in a
   permanently broken state.

   Note the existing variable scoping: `err` is already declared earlier in
   the same generated function block (from the NATS connection / JetStream
   context setup), so the generated statement should be `_, err =
   js.AddConsumer(...)` (plain assignment, not `:=`), followed by the error
   check block — mirror exactly how the codebase already reassigns `err` in
   nearby blocks.

2. In the same file, make `/readyz` reflect actual subscription health
   instead of a static 200. The generated code has access to the consumer's
   stream/consumer name after the fix in (1); use `js.ConsumerInfo(stream,
   consumer)` (or track a simple readiness flag set once `AddConsumer` +
   `PullSubscribe` both succeed, and cleared if the reconcile loop detects a
   permanent subscription error) and return 503 if the check fails. Keep this
   scoped — don't add unrelated health dimensions.

3. In `pkg/controller/v0/reconcile.go`, `PullMessage`: distinguish a
   structural/permanent fetch error (e.g. "consumer not found" — the consumer
   object no longer exists server-side) from a transient one. On a permanent
   error, do not just log-and-continue forever; escalate (e.g. `os.Exit(1)`
   so Kubernetes restarts the pod and the startup path in (1)/(2) gets a
   chance to recreate the consumer). Do not change behavior for
   `nats.ErrTimeout` or other genuinely transient errors — those should keep
   retrying as they do today.

4. Regenerate (or manually apply the equivalent generated-code change to)
   every existing `main_gen.go` produced by this generator in `threeport/0.7`,
   so the fix is live immediately and not just in the template. As of this
   writing, the affected files are:
   ```
   cmd/control-plane-controller/main_gen.go
   cmd/secret-controller/main_gen.go
   cmd/gcp-controller/main_gen.go
   cmd/aws-controller/main_gen.go
   cmd/oci-controller/main_gen.go
   cmd/kubernetes-runtime-controller/main_gen.go
   cmd/kubernetes-workload-controller/main_gen.go
   cmd/gateway-controller/main_gen.go
   cmd/observability-controller/main_gen.go
   cmd/terraform-controller/main_gen.go
   cmd/machine-runtime-controller/main_gen.go
   cmd/machine-workload-controller/main_gen.go
   cmd/helm-workload-controller/main_gen.go
   ```
   Check how codegen is normally invoked in this repo (there's a `magefiles/`
   directory and a `cmd/sdk` CLI) before hand-editing generated files —
   regenerating is preferable to manual edits so the generator and its output
   stay in sync.

### Part 2 — propagate the fix to `qleet-threeport-module`

The dev-18 environment actually runs controllers built from
`qleet-threeport-module`, not from a local checkout of `threeport/0.7`
directly. Its `go.mod` pins `github.com/threeport/threeport` to a specific
released/tagged version (a pseudo-version, not a local path replace), so a
fix made only in a local `threeport/0.7` checkout will **not** automatically
reach this module's generated output.

Affected generated files in `qleet-threeport-module` (same defect, same
generator dependency):
```
cmd/organizations-controller/main_gen.go
cmd/users-controller/main_gen.go
cmd/roles-controller/main_gen.go
cmd/policies-controller/main_gen.go
cmd/control-planes-controller/main_gen.go
```

To actually get this fix onto dev-18, do one of:

- **Preferred**: once the fix in Part 1 is merged and released/tagged in
  `threeport/0.7`, bump the `github.com/threeport/threeport` dependency
  version in `qleet-threeport-module/go.mod` and re-run this module's own
  codegen (`cmd/sdk`) to regenerate its 5 controller `main_gen.go` files
  against the fixed generator.
- **Interim/if a release cycle isn't available yet**: manually apply the
  equivalent hand-edit (same three changes as Part 1, items 1-3) directly to
  the 5 generated files listed above. Treat this as a stopgap — flag clearly
  in the PR description that these files should be regenerated for real once
  the dependency bump is possible, so they don't silently drift from what the
  generator would now produce.

After either approach, these controllers need to be rebuilt and redeployed to
dev-18 for the fix to take effect there (out of scope for this document to
execute, but flag it as the remaining deployment step).

## Out of scope — do not address in this task

- **The APISIX gateway etcd-sync issue**: a separate, unrelated bug where the
  gateway's in-memory routing table can fall out of sync with its etcd config
  store. This was worked around operationally (pod restart) on dev-18 but the
  underlying cause is still unknown and untouched. Do not conflate it with
  this NATS consumer issue.
- **A GKE IAM/RBAC permissions gap** found in `control-plane-controller`
  (missing `container.namespaces.create`-equivalent permissions for
  provisioning brand-new child control planes). Unrelated to this defect;
  tracked separately.
- **Any live changes to the dev-18 cluster** (pod restarts, RBAC grants,
  etc.) — dev-18 is already in a working state via manual operational fixes.
  This task is code-only, targeting a future redeploy/release, not an
  emergency fix to the running environment.

## Acceptance criteria

- A fresh `main_gen.go` generated from the fixed template, for any object
  type, calls `os.Exit(1)` (with a logged error) if `AddConsumer` fails,
  matching the existing `PullSubscribe` error-handling convention.
- `/readyz` on a controller whose consumer failed to bind returns a non-200
  status, not a static `200 OK`.
- The reconcile loop's `PullMessage` escalates (rather than silently
  retrying forever) on a structural/permanent consumer error, while still
  handling ordinary `nats.ErrTimeout` as routine.
- All 13 `threeport/0.7` generated controllers and all 5
  `qleet-threeport-module` generated controllers reflect the fix (via
  regeneration, or an explicitly-flagged interim manual edit for the latter).
- No changes made to the APISIX gateway sync issue, the GKE IAM/RBAC issue,
  or live dev-18 cluster state as part of this task.

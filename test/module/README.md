# Module generation test

A Threeport module generated from one config file, used to check that the SDK's
module code path still compiles and still runs.

## Why it exists

`threeport-sdk gen` emits different code for a module than for this repository.
It decides which by reading the module path from `go.mod`: any path other than
`github.com/threeport/threeport` is a module.

So generating for this repository never emits the module code. That code calls
this repository's exported API by name. Change a signature and every check here
stays green, while the break waits in a module repository, far from the commit
that caused it.

## What is tracked

Two files: `sdk-config.yaml` and this one.

Everything else is generated, including the API types, which
`threeport-sdk create` scaffolds from `sdk-config.yaml`. A checked-in copy would
go stale against the generator it is meant to exercise.

A successful run deletes what it generated, so this directory holds two files at
rest. A failed run leaves the generated source for you to read. Git ignores it
either way.

The module registers as `module.threeport.io/test-module-api`. The generator
builds that name as `<ApiNamespace>/<ModuleName>-module-api`, with the suffix
fixed.

## mage test:moduleGen

Generates the module and compiles it. No cluster needed.

1. Rebuilds `threeport-sdk` from this checkout. It installs to one global path,
   so the binary on `PATH` may have come from another worktree.
2. Deletes everything here except the tracked files. This forces the SDK to
   re-emit scaffolding, which it otherwise writes only once.
3. Writes a `go.mod`. The module path selects the module code path; the
   `replace` directive points the generated code at this checkout.
4. Runs `threeport-sdk create`, then `threeport-sdk gen`.
5. Runs `go mod tidy`.
6. Builds every generated package. Vets `magefiles`, which cannot be built
   because `mage` supplies its `main` function at run time.
7. Deletes what it generated.

## mage test:moduleInstall

Generates the module, builds and pushes its images, and installs it into the
control plane named by the local Threeport config. Covers what compiling cannot:
migrations apply, the module registers with the control plane, and its routes
answer through the control plane's proxy.

Needs a running control plane and a registry the cluster can pull from.

## Continuous integration

| Job | Target | Needs |
|---|---|---|
| `Unit Tests` | `mage test:moduleGen` | Nothing |
| `Integration Tests` | `mage test:moduleInstall` | Control plane, registry |

`mage test:moduleGen` runs in the unit job because it compiles a second Go
module and needs no cluster.  `mage test:moduleInstall` runs after the
integration tests, on the control plane that job already created.

## What it catches

An exported symbol that generated module code calls, disappearing or changing
shape. A generator change that emits code a module cannot compile. A module that
fails to install or register.

## What it does not catch

Breaks in hand-written module code, which calls parts of the API this module
never touches. Breaks that only appear on upgrade, since every run installs
fresh. Breaks against a published release, since the `replace` directive always
points at the working tree.

## Changing it

Add an object to `sdk-config.yaml` when a generator change only shows up for a
kind of object not yet declared. Keep the set small. Every object is compiled on
every run, and the target covers the code path, not the object model.

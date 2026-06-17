# Chat History — Threeport Docs Workload Rename

## Turn 1 — User

> The requirements for the next change are in: @ai/requirements.md Review the file and propose changes for me to approve.

_(The assistant read `ai/requirements.md`, which contained:)_

```
# Requirements for Updating Workload Names in Threeport Docs

## update 1
We need to update all instances of the word "Workload" only when it is talking
about a Defined Instance Abstraction (This term is defined in
`docs/docs/concepts/definitions-instances.md`). The new term should be
"KubernetesWorkload".

## update 2
ALL instances of the term "WorkloadDefinition" ---> "KubernetesWorkloadDefinition"
ALL instances of the term "WorkloadInstance" ---> "KubernetesWorkloadDefinition"

Note: All svg diagrams in the docs should be updated too if needed

All docs are in `docs`.
```

---

## Turn 2 — Assistant

Explored the `docs` directory to find all occurrences of `WorkloadDefinition`, `WorkloadInstance`, and standalone `Workload` across `.md` and `.svg` files.

**Findings:**

| Term | Files | Occurrences |
|---|---|---|
| `WorkloadDefinition` | 10 files | ~31 |
| `WorkloadInstance` | 10 files | ~26 |
| `Workload` (standalone) | 41 files | 120+ |

**Flagged a typo in the requirements:**

> `requirements.md:9` says `"WorkloadInstance"` → `"KubernetesWorkloadDefinition"`
> This almost certainly should be `"KubernetesWorkloadInstance"` — did you mean that?

**Proposed plan:**

- **Update 2 & 3 (mechanical):** Global rename of `WorkloadDefinition` → `KubernetesWorkloadDefinition` and `WorkloadInstance` → `KubernetesWorkloadInstance` across 10 files each, including two SVG diagrams.
- **Update 1 (contextual):** Rename standalone `"Workload"` → `"KubernetesWorkload"` only when used as the Defined Instance Abstraction type name (e.g. `Workload:` YAML key, "the Workload objects"), not when used generically (page titles, link text, lowercase usage).

---

## Turn 3 — User

> Yes you are right that was a typo. You may proceed.

---

## Turn 4 — Assistant

**Research phase:**

Read the following files to understand context and identify every DIA vs generic usage:
- `docs/docs/concepts/definitions-instances.md`
- `docs/docs/workloads/workload-intro.md`
- `docs/docs/workloads/deploy-workload-local.md`
- `docs/docs/workloads/namespaces.md`
- `docs/docs/helm-workloads/deploy-helm-local.md`
- `docs/docs/helm-workloads/helm-intro.md`
- `docs/docs/concepts/attached-object-reference.md`
- `docs/docs/sdk/struct-tags.md`
- `docs/docs/getting-started.md`
- `docs/docs/architecture/threeport-controllers.md`
- `docs/docs/architecture/control-plane.md`
- `docs/dev/threeport-controllers.md`
- `docs/docs/concepts/extensions.md` (partial)
- `docs/docs/architecture/threeport-core.md` (partial)
- `docs/docs/img/ThreeportDevExperience.drawio.svg` (partial)
- `docs/docs/img/DefinedInstanceAbstraction.drawio.svg` (partial)

Ran multiple `grep` passes to:
1. Confirm which files had bare (un-prefixed) `WorkloadDefinition`/`WorkloadInstance`
2. List all standalone `\bWorkload\b` occurrences
3. Check SVG label styles (camelCase vs spaced human-readable)

**Key findings:**

- Most `WorkloadDefinition`/`WorkloadInstance` occurrences in `.md` files were already `KubernetesWorkload*` or `HelmWorkload*` — only `ThreeportReconcilers.drawio.svg` had truly bare camelCase labels.
- Standalone DIA `Workload` usages in markdown: `deploy-workload-local.md` (YAML key + prose), `control-plane.md`, `extensions.md`, `threeport-core.md`.
- SVG diagrams requiring label updates: `ThreeportReconcilers`, `DefinedInstanceAbstraction`, `DefinitionsInstances`, `ThreeportForDevOps`.
- `ThreeportDevExperience.drawio.svg` "Workload" = "Front End Workload" compound label — generic, not DIA.

**Changes executed:**

### Markdown files

| File | Change |
|---|---|
| `docs/docs/workloads/deploy-workload-local.md` | YAML key `Workload:` → `KubernetesWorkload:` |
| `docs/docs/workloads/deploy-workload-local.md` | `## Create Workload` → `## Create KubernetesWorkload` |
| `docs/docs/workloads/deploy-workload-local.md` | "create the Workload objects" → "create the KubernetesWorkload objects" |
| `docs/docs/architecture/control-plane.md` | "provided to the Workload is" → "provided to the KubernetesWorkload is" |
| `docs/docs/architecture/control-plane.md` | "Workload resource to configure" → "KubernetesWorkload resource to configure" |
| `docs/docs/concepts/extensions.md` | "create a single Workload object" → "create a single KubernetesWorkload object" |
| `docs/docs/architecture/threeport-core.md` | "Workload objects in the Threeport API" → "KubernetesWorkload objects in the Threeport API" |

### SVG files

| File | Change |
|---|---|
| `docs/docs/img/ThreeportReconcilers.drawio.svg` | `WorkloadInstance` → `KubernetesWorkloadInstance` (×2) |
| `docs/docs/img/ThreeportReconcilers.drawio.svg` | `WorkloadDefinition` → `KubernetesWorkloadDefinition` (×2) |
| `docs/docs/img/DefinedInstanceAbstraction.drawio.svg` | `Workload Instance` → `Kubernetes Workload Instance` (×8) |
| `docs/docs/img/DefinedInstanceAbstraction.drawio.svg` | `Workload` → `Kubernetes Workload` (×2, DIA label) |
| `docs/docs/img/DefinitionsInstances.drawio.svg` | `Workload Instance Name: my-app-*` → `Kubernetes Workload Instance Name: my-app-*` (×4) |
| `docs/docs/img/DefinitionsInstances.drawio.svg` | `Workload Definition` → `Kubernetes Workload Definition` (×2) |
| `docs/docs/img/ThreeportForDevOps.drawio.svg` | `Workload Instance` → `Kubernetes Workload Instance` (×2) |
| `docs/docs/img/ThreeportForDevOps.drawio.svg` | `Workload Definition` → `Kubernetes Workload Definition` (×2) |
| `docs/docs/img/ThreeportForDevOps.drawio.svg` | `Workload` → `Kubernetes Workload` (×2, DIA label) |

**Intentionally not changed:**
- Generic prose: "deploy a workload", "workload controller", "workload delivery"
- `HelmWorkload*` and `Machine Workload` references (different types, already prefixed)
- Page titles, link text, section headings referring to the action/topic area
- `Workload controller` label in `ThreeportReconcilers.drawio.svg` (component name, not DIA type)
- Occurrences already reading "Kubernetes Workload Definition / Instance" (human-readable, already correct)

---

## Turn 5 — User

> Can you generate a full history of this chat in a file named 'chat.md'. put it in the `ai` directory.

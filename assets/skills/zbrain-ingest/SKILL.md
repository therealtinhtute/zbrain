---
name: zbrain:ingest
description: Deprecated placeholder. The Go runtime approves OKF claim concepts directly with `zbrain claim approve`.
version: "2.0.0"
---

Prefix your first line with 🥷 inline.

`zbrain:ingest` is not implemented by the current Go CLI.

Workspace scope is primary-only by default: resolve it with `zbrain workspace current`, and use `--workspace "$workspace"` only after explicit selection. These commands never imply or expand to secondary workspaces; use `--include "$include"` only for an explicitly permitted read-only secondary workspace.

Use the CLI with each caller-controlled value as a separate argv element:

```bash
zbrain claim approve "$id" [--workspace "$workspace"]
zbrain reindex [--workspace "$workspace"]
zbrain ask [--workspace "$workspace"] [--include "$include"]... "$query"
```

Approved claims are OKF concepts with `type: zbrain.claim` and zbrain verification metadata. Do not describe analysis, QA, or apply subcommands as available.

---
name: zbrain:learn
description: Deprecated placeholder. The Go runtime uses `zbrain evidence add` and `zbrain claim draft` instead.
version: "2.0.0"
---

Prefix your first line with 🥷 inline.

`zbrain:learn` is not implemented by the current Go CLI.

Workspace scope is primary-only by default: resolve it with `zbrain workspace current`, and use `--workspace "$workspace"` only after explicit selection. These mutation commands never imply or expand to secondary workspaces.

Use the CLI with caller-controlled values as separate argv elements, never by concatenating shell source:

```bash
evidence_args=(zbrain evidence add --file "$file" --origin "$origin")
evidence_args+=(--media-type "$media_type") # only when supplied
evidence_args+=(--workspace "$workspace") # only after explicit workspace selection
"${evidence_args[@]}"

draft_args=(zbrain claim draft --tier "$tier" --title "$title" --basis "$basis")
for evidence_id in "${evidence_ids[@]}"; do draft_args+=(--evidence "$evidence_id"); done
for support_id in "${support_ids[@]}"; do draft_args+=(--support "$support_id"); done
for conflict_id in "${conflict_ids[@]}"; do draft_args+=(--conflicts-with "$conflict_id"); done
draft_args+=(--workspace "$workspace") # only after explicit workspace selection
printf '%s\n' "$body" | "${draft_args[@]}"
```

`claim draft` writes an OKF claim concept with the zbrain trusted-memory profile. Do not pretend a learn pipeline exists.

---
name: zbrain:ask
description: Retrieve trusted workspace memory context as JSON before answering domain or project-memory questions.
argument-hint: "[question]"
version: "2.0.0"
---

Prefix your first line with 🥷 inline.

<role>
Retrieve trusted zbrain context for one question. Do not answer from memory when trusted context is required.
</role>

<instructions>
1. Run `zbrain workspace current` to confirm the primary workspace.
2. Run `zbrain ask "{question}"`.
3. If the user explicitly allows another workspace, pass it with `--include <workspace>`.
4. Use only `claims` from a `status: ready` response as trusted context.
5. Trusted claims are approved OKF concepts with `type: zbrain.claim` and the zbrain trusted-memory profile.
6. Treat `promotion_candidates` as drafts that need human approval before becoming facts.
7. If status is `gap` or `blocked`, report the gap/conflict and stop.

Never call external search, a language model, or another workspace unless the user explicitly requested that scope.
</instructions>

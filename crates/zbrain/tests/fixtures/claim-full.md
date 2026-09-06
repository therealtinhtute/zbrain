---
type: zbrain.claim
title: Trusted Ask Contract
description: Trusted ask returns scoped context.
resource: https://example.com/trusted-ask
tags:
    - memory
    - trust
sources:
    - id: evd_0123456789abcdef0123456789abcdef
      resource: evidence/sources/evd_0123456789abcdef0123456789abcdef/raw
      title: file://source.txt
      digest: sha256:evidence-v1:0123456789abcdef0123456789abcdef0123456789abcdef0123456789abcdef
      spans:
        - evidence_id: evd_0123456789abcdef0123456789abcdef
          start_line: 2
          end_line: 3
          digest: sha256:span-v1:0123456789abcdef0123456789abcdef0123456789abcdef0123456789abcdef
generated:
    at: "2026-07-30T09:00:00Z"
    by: owner
verified:
    at: "2026-07-30T10:00:00Z"
    by: owner
    digest: sha256:0123456789abcdef0123456789abcdef0123456789abcdef0123456789abcdef
status: approved
stale_after: "2027-07-30T09:00:00Z"
zbrain:
    profile: zbrain.trusted-memory/v1
    id: clm_0123456789abcdef0123456789abcdef
    tier: projects
    basis: evidence
    evidence_ids:
        - evd_0123456789abcdef0123456789abcdef
    supporting_claim_ids:
        - clm_11111111111111111111111111111111
    supersedes:
        - clm_22222222222222222222222222222222
    conflicts_with:
        - clm_33333333333333333333333333333333
    contradicts:
        - claim_id: clm_44444444444444444444444444444444
          heuristic: value_swap
    transitions:
        - kind: approve
          at: "2026-07-30T10:00:00Z"
          by: owner
        - kind: supersede
          at: "2026-07-30T11:00:00Z"
          by: owner
          reason: corrected scope
          related_claim_ids:
            - clm_bbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbb
          prior_verification_digest: sha256:0123456789abcdef0123456789abcdef0123456789abcdef0123456789abcdef
          authorization:
            challenge_id: chg_0123456789abcdef0123456789abcdef
            method: claim_lifecycle.apply
            mcp_client: mcp-client/1.0
---
# Body

Keep this exact markdown body.

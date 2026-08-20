# Documentation

The authored documentation map for this repository. Every hand-written document
should be reachable from this page.

`zharness init` wrote this file once, because it was absent. It is yours now —
the harness never refreshes, overwrites, or deletes it.

## Where to go

| You want to | Read |
|---|---|
| Run a workflow stage | docs/WORKFLOW.md, then the one playbook it names |
| Know why something is built the way it is | docs/decisions/README.md |
| See what is being built right now | the active plan under docs/plans/active/ |

Add a row for each authored document as you write it.

## Ownership

Three classes. The class decides who is allowed to edit the file.

- **managed** — projected from the binary's embedded doc set and hash-tracked.
  Edit the embedded source and cut a release; a local edit is staged under
  .kit/conflicts/ rather than silently overwritten. Covers docs/WORKFLOW.md and
  docs/playbooks/.
- **scaffold-once** — written by `zharness init` only when absent, then owned by
  you. Covers this file, docs/decisions/README.md, and
  docs/decisions/templates/decision.md.
- **authored** — written by hand, never embedded, never regenerated. Everything
  else under docs/.

An existing path under docs/ that is missing from this page is a defect in this
page.

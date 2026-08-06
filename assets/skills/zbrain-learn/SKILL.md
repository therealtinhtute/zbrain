---
name: zbrain:learn
description: Deprecated placeholder. The Go runtime uses `zbrain evidence add` and `zbrain claim draft` instead.
version: "2.0.0"
---

Prefix your first line with 🥷 inline.

`zbrain:learn` is not implemented by the current Go CLI.

Use:

```bash
zbrain evidence add --file <path> --origin <uri-or-path> [--media-type <type>] [--workspace <name>]
printf '<claim body>\n' | zbrain claim draft --tier <tier> --title '<title>' --basis <owner|evidence|derived> [--evidence <id>]... [--support <id>]... [--conflicts-with <id>]... [--workspace <name>]
```

`claim draft` writes an OKF claim concept with the zbrain trusted-memory profile. Do not pretend a learn pipeline exists.

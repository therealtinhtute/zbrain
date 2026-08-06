# Deprecated Pipeline Reference

The previous learn/ingest/apply pipeline is not implemented in the current Go runtime.

Current supported flow:

1. `zbrain evidence add --file <path> --origin <uri-or-path> [--media-type <type>] [--workspace <name>]`
2. `zbrain claim draft --tier <tier> --title <title> --basis <owner|evidence|derived> [--evidence <id>]... [--support <id>]... [--conflicts-with <id>]... [--workspace <name>]`
3. `zbrain claim approve <id> [--workspace <name>]`
4. `zbrain reindex [--workspace <name>]`
5. `zbrain ask [--workspace <name>] [--include <name>]... <query>`

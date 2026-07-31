# Deprecated Pipeline Reference

The previous learn/ingest/apply pipeline is not implemented in the current Go runtime.

Current supported flow:

1. `zbrain evidence add --file <path> --origin <uri-or-path>`
2. `zbrain claim draft --tier <tier> --title <title> --basis <owner|evidence|derived>`
3. `zbrain claim approve <id>`
4. `zbrain reindex`
5. `zbrain ask <query>`

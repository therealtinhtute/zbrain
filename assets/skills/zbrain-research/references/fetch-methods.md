# Deprecated Fetch Methods Reference

Network fetching is outside the current Go trusted-memory slice.

Current supported flow uses already-local source material only:

1. Save or export source material to a local file.
2. Capture it with `zbrain evidence add --file <path> --origin <uri-or-path> [--media-type <type>] [--workspace <name>]`.
3. Draft an evidence-backed claim with `zbrain claim draft --tier <tier> --title <title> --basis evidence --evidence <id> [--workspace <name>]`.
4. Approve the claim with `zbrain claim approve <id> [--workspace <name>]`.
5. Rebuild retrieval with `zbrain reindex [--workspace <name>]`.

Do not fetch, crawl, or proxy remote content from this runtime asset.

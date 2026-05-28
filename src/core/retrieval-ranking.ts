import { type QmdSearchResult } from "./qmd-adapter";

export type RetrievalTier = "P0" | "P1" | "P2" | "P3";

export interface RankedRetrievalResult extends QmdSearchResult {
  tier: RetrievalTier;
  workspace?: string;
}

const tierOrder: Record<RetrievalTier, number> = {
  P0: 0,
  P1: 1,
  P2: 2,
  P3: 3,
};

export function classifyByTier(path: string): RetrievalTier {
  const normalized = path.replace(/\\/g, "/");
  if (normalized.includes("/axioms/")) {
    return "P0";
  }
  if (normalized.includes("/mental-models/")) {
    return "P1";
  }
  if (normalized.includes("/projects/")) {
    return "P2";
  }
  if (normalized.includes("/decisions/")) {
    return "P3";
  }
  return "P2";
}

export function rankRetrievalResults(results: QmdSearchResult[], limit = 8): RankedRetrievalResult[] {
  return results
    .map((result, index) => ({
      ...result,
      tier: classifyByTier(result.path),
      _index: index,
    }))
    .sort((left, right) => {
      const tierDiff = tierOrder[left.tier] - tierOrder[right.tier];
      if (tierDiff !== 0) {
        return tierDiff;
      }

      return left._index - right._index;
    })
    .slice(0, limit)
    .map(({ _index, ...result }) => result);
}

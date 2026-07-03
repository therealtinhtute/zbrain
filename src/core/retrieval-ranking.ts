import { type QmdSearchResult } from "./qmd-adapter";
import { TIER_WEIGHTS } from "../adapters/retrieval/fts5-adapter";

export type RetrievalTier = "P0" | "P1" | "P2" | "P3";

export interface RankedRetrievalResult extends QmdSearchResult {
  tier: RetrievalTier;
  workspace?: string;
  weightedScore?: number;
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

function tierWeight(tier: RetrievalTier): number {
  return TIER_WEIGHTS[
    tier === "P0" ? "axioms" : tier === "P1" ? "mental-models" : tier === "P2" ? "projects" : "decisions"
  ] ?? 1.0;
}

// V2: tier-weighted scoring. `BM25 × tier_weight`. Preserves priority while
// protecting relevance — a strongly-relevant decision can outrank a
// weakly-relevant axiom.
export function rankRetrievalResults(results: QmdSearchResult[], limit = 8): RankedRetrievalResult[] {
  const safeLimit = Number.isInteger(limit) && limit > 0 ? limit : 8;
  return results
    .map((result, index) => {
      const tier = classifyByTier(result.path);
      const weight = tierWeight(tier);
      return {
        ...result,
        tier,
        weightedScore: result.score * weight,
        _index: index,
      };
    })
    .sort((left, right) => {
      if ((left.weightedScore ?? 0) !== (right.weightedScore ?? 0)) {
        return (right.weightedScore ?? 0) - (left.weightedScore ?? 0);
      }
      const tierDiff = tierOrder[left.tier] - tierOrder[right.tier];
      if (tierDiff !== 0) return tierDiff;
      if (left.score !== right.score) return right.score - left.score;
      return left._index - right._index;
    })
    .slice(0, safeLimit)
    .map(({ _index, ...result }) => result);
}

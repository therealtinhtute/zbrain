//! eval.rs — port of the internal/eval six-track suite's pure evaluation
//! math: retrieval metrics (P/R/MRR/NDCG/AP, from eval.go) and the drift
//! comparison (ΔP/ΔR + McNemar paired chi², from drift.go).
//!
//! Corpus runners live in integration tests (tests/eval_*.rs); this module
//! holds the deterministic, clock-free computation both languages share.

use serde::{Deserialize, Serialize};

#[derive(Debug, Clone, Default, Serialize, Deserialize)]
pub struct PerQuery {
    pub id: String,
    pub text: String,
    pub precision: f64,
    pub recall: f64,
    pub mrr: f64,
    pub ndcg: f64,
    pub ap: f64,
    pub relevant: usize,
    pub retrieved: usize,
    pub hits: usize,
    pub gap: bool,
    pub blocked: bool,
    pub rank_first: usize,
}

#[derive(Debug, Clone, Default, Serialize, Deserialize)]
pub struct EvalSummary {
    pub corpus_size: usize,
    pub limit: usize,
    pub total_queries: usize,
    pub precision_at_k: f64,
    pub recall_at_k: f64,
    pub f1_at_k: f64,
    pub mrr: f64,
    pub ndcg_at_k: f64,
    pub map_at_k: f64,
    pub gap_rate: f64,
    pub blocked_rate: f64,
    pub faithfulness: f64,
    pub queries: Vec<PerQuery>,
}

/// Score one query: binary relevance (retrieved id ∈ relevant set).
pub fn score_query(
    id: &str,
    text: &str,
    retrieved_ids: &[String],
    relevant_ids: &[String],
    limit: usize,
) -> PerQuery {
    let relevant: std::collections::HashSet<&str> =
        relevant_ids.iter().map(String::as_str).collect();
    let mut hits = 0;
    for candidate in retrieved_ids {
        if relevant.contains(candidate.as_str()) {
            hits += 1;
        }
    }
    let precision = if retrieved_ids.is_empty() {
        0.0
    } else {
        hits as f64 / retrieved_ids.len() as f64
    };
    let recall = if relevant_ids.is_empty() {
        0.0
    } else {
        hits as f64 / relevant_ids.len() as f64
    };
    let mut mrr = 0.0;
    let mut rank_first = 0;
    for (index, candidate) in retrieved_ids.iter().enumerate() {
        if relevant.contains(candidate.as_str()) {
            mrr = 1.0 / (index + 1) as f64;
            rank_first = index + 1;
            break;
        }
    }
    let mut sum_prec = 0.0;
    let mut seen = 0;
    for (index, candidate) in retrieved_ids.iter().enumerate() {
        if relevant.contains(candidate.as_str()) {
            seen += 1;
            sum_prec += seen as f64 / (index + 1) as f64;
        }
    }
    let mut denom = relevant_ids.len();
    if denom > limit {
        denom = limit;
    }
    let ap = if denom > 0 && hits > 0 {
        sum_prec / denom as f64
    } else {
        0.0
    };
    let ndcg = ndcg_at_k(retrieved_ids, &relevant, limit);
    PerQuery {
        id: id.to_string(),
        text: text.to_string(),
        precision,
        recall,
        mrr,
        ndcg,
        ap,
        relevant: relevant_ids.len(),
        retrieved: retrieved_ids.len(),
        hits,
        gap: retrieved_ids.is_empty(),
        blocked: false,
        rank_first,
    }
}

pub fn ndcg_at_k(retrieved: &[String], relevant: &std::collections::HashSet<&str>, k: usize) -> f64 {
    if retrieved.is_empty() || relevant.is_empty() {
        return 0.0;
    }
    let mut dcg = 0.0;
    for (index, id) in retrieved.iter().enumerate() {
        if index >= k {
            break;
        }
        if relevant.contains(id.as_str()) {
            dcg += 1.0 / ((index + 2) as f64).log2();
        }
    }
    let mut relevant_count = relevant.len();
    if relevant_count > k {
        relevant_count = k;
    }
    let mut idcg = 0.0;
    for index in 0..relevant_count {
        idcg += 1.0 / ((index + 2) as f64).log2();
    }
    if idcg == 0.0 {
        return 0.0;
    }
    dcg / idcg
}

/// Aggregate per-query scores into a summary (means + F1 + rates).
pub fn summarize(
    corpus_size: usize,
    limit: usize,
    per: &[PerQuery],
    blocked: usize,
    faithfulness: f64,
) -> EvalSummary {
    let n = per.len().max(1) as f64;
    let precision: f64 = per.iter().map(|q| q.precision).sum::<f64>() / n;
    let recall: f64 = per.iter().map(|q| q.recall).sum::<f64>() / n;
    let mrr: f64 = per.iter().map(|q| q.mrr).sum::<f64>() / n;
    let ndcg: f64 = per.iter().map(|q| q.ndcg).sum::<f64>() / n;
    let map: f64 = per.iter().map(|q| q.ap).sum::<f64>() / n;
    let gaps = per.iter().filter(|q| q.gap).count();
    let mut summary = EvalSummary {
        corpus_size,
        limit,
        total_queries: per.len(),
        precision_at_k: precision,
        recall_at_k: recall,
        f1_at_k: 0.0,
        mrr,
        ndcg_at_k: ndcg,
        map_at_k: map,
        gap_rate: gaps as f64 / n,
        blocked_rate: blocked as f64 / n,
        faithfulness,
        queries: per.to_vec(),
    };
    if precision + recall > 0.0 {
        summary.f1_at_k = 2.0 * precision * recall / (precision + recall);
    }
    summary.queries.sort_by(|a, b| a.id.cmp(&b.id));
    summary
}

#[derive(Debug, Clone, Default, Serialize)]
pub struct DriftDelta {
    pub precision: f64,
    pub recall: f64,
    pub f1: f64,
    pub mrr: f64,
    pub ndcg: f64,
    pub map: f64,
    pub gap_rate: f64,
    pub blocked_rate: f64,
    pub faithfulness: f64,
}

#[derive(Debug, Clone, Default, Serialize)]
pub struct McNemar {
    pub a_both_hit: usize,
    pub b_before_only: usize,
    pub c_after_only: usize,
    pub d_both_miss: usize,
    pub total_paired: usize,
    pub discordant: usize,
    pub chi2: f64,
    pub chi2_cc: f64,
    pub p_value: f64,
    pub p_value_cc: f64,
    pub significant: bool,
    pub feasible: bool,
    pub note: String,
}

pub fn drift_delta(before: &EvalSummary, after: &EvalSummary) -> DriftDelta {
    DriftDelta {
        precision: after.precision_at_k - before.precision_at_k,
        recall: after.recall_at_k - before.recall_at_k,
        f1: after.f1_at_k - before.f1_at_k,
        mrr: after.mrr - before.mrr,
        ndcg: after.ndcg_at_k - before.ndcg_at_k,
        map: after.map_at_k - before.map_at_k,
        gap_rate: after.gap_rate - before.gap_rate,
        blocked_rate: after.blocked_rate - before.blocked_rate,
        faithfulness: after.faithfulness - before.faithfulness,
    }
}

pub fn drift_detected(delta: &DriftDelta, threshold: f64) -> (bool, String) {
    let mut reasons = Vec::new();
    if delta.precision.abs() > threshold {
        reasons.push(format!("ΔP={:.3} > {:.1}%", delta.precision, threshold * 100.0));
    }
    if delta.recall.abs() > threshold {
        reasons.push(format!("ΔR={:.3} > {:.1}%", delta.recall, threshold * 100.0));
    }
    (!reasons.is_empty(), reasons.join("; "))
}

/// McNemar paired test on per-query hit/miss (hits>0 = hit), keyed by query
/// id over the intersection of both runs.
pub fn mcnemar(before: &[PerQuery], after: &[PerQuery]) -> McNemar {
    let before_map: std::collections::HashMap<&str, &PerQuery> =
        before.iter().map(|q| (q.id.as_str(), q)).collect();
    let after_map: std::collections::HashMap<&str, &PerQuery> =
        after.iter().map(|q| (q.id.as_str(), q)).collect();
    let mut paired: Vec<(&PerQuery, &PerQuery)> = Vec::new();
    for (id, before_q) in &before_map {
        if let Some(after_q) = after_map.get(id) {
            paired.push((before_q, after_q));
        }
    }
    let (mut a, mut b, mut c, mut d) = (0usize, 0usize, 0usize, 0usize);
    for (before_q, after_q) in &paired {
        match (before_q.hits > 0, after_q.hits > 0) {
            (true, true) => a += 1,
            (true, false) => b += 1,
            (false, true) => c += 1,
            (false, false) => d += 1,
        }
    }
    let mut result = McNemar {
        a_both_hit: a,
        b_before_only: b,
        c_after_only: c,
        d_both_miss: d,
        total_paired: paired.len(),
        discordant: b + c,
        ..McNemar::default()
    };
    if b + c > 0 {
        result.feasible = true;
        result.chi2 = (b as f64 - c as f64).powi(2) / (b + c) as f64;
        result.p_value = chi2_p_value_1df(result.chi2);
        let diff = (b as f64 - c as f64).abs();
        result.chi2_cc = if diff > 0.0 {
            (diff - 1.0).powi(2) / (b + c) as f64
        } else {
            0.0
        };
        result.p_value_cc = chi2_p_value_1df(result.chi2_cc);
        result.significant = result.p_value < 0.05 || result.p_value_cc < 0.05;
        result.note = if result.significant {
            "significant paired difference (p<0.05)".to_string()
        } else {
            "no significant paired difference (p>=0.05)".to_string()
        };
    } else {
        result.feasible = false;
        result.p_value = 1.0;
        result.p_value_cc = 1.0;
        result.note = if paired.is_empty() {
            "no paired queries; per-query delta only".to_string()
        } else {
            "no discordant pairs (b+c=0); chi2 not computable, use simple delta".to_string()
        };
    }
    result
}

/// p-value for chi-squared with 1 degree of freedom via the complementary
/// error function: p = erfc(sqrt(chi2/2)).
pub fn chi2_p_value_1df(chi2: f64) -> f64 {
    if chi2 <= 0.0 {
        return 1.0;
    }
    erfc((chi2 / 2.0).sqrt())
}

fn erf(x: f64) -> f64 {
    // Abramowitz-Stegun 7.1.26; |error| <= 1.5e-7, plenty for a drift gate.
    let sign = if x < 0.0 { -1.0 } else { 1.0 };
    let x = x.abs();
    let t = 1.0 / (1.0 + 0.3275911 * x);
    let y = 1.0
        - (((((1.061405429 * t - 1.453152027) * t) + 1.421413741) * t - 0.284496736) * t
            + 0.254829592)
            * t
            * (-x * x).exp();
    sign * y
}

fn erfc(x: f64) -> f64 {
    1.0 - erf(x)
}

#[cfg(test)]
mod tests {
    use super::*;

    #[test]
    fn perfect_single_hit_scores_one() {
        let relevant = vec!["clm_1".to_string()];
        let retrieved = vec!["clm_1".to_string()];
        let scored = score_query("q1", "text", &retrieved, &relevant, 10);
        assert_eq!(scored.precision, 1.0);
        assert_eq!(scored.recall, 1.0);
        assert_eq!(scored.mrr, 1.0);
        assert_eq!(scored.ndcg, 1.0);
        assert_eq!(scored.ap, 1.0);
        assert!(!scored.gap);
    }

    #[test]
    fn empty_retrieval_is_gap_with_zero_metrics() {
        let relevant = vec!["clm_1".to_string()];
        let scored = score_query("q1", "text", &[], &relevant, 10);
        assert!(scored.gap);
        assert_eq!(scored.precision, 0.0);
        assert_eq!(scored.ndcg, 0.0);
    }

    #[test]
    fn mcnemar_counts_and_chi2_match_drift_oracle() {
        // b=1 before-only hit, c=3 after-only hits: chi2 = (1-3)^2/4 = 1.
        let before = vec![
            PerQuery { id: "a".into(), hits: 1, ..PerQuery::default() },
            PerQuery { id: "b".into(), hits: 1, ..PerQuery::default() },
            PerQuery { id: "c".into(), hits: 0, ..PerQuery::default() },
            PerQuery { id: "d".into(), hits: 0, ..PerQuery::default() },
            PerQuery { id: "e".into(), hits: 0, ..PerQuery::default() },
        ];
        let after = vec![
            PerQuery { id: "a".into(), hits: 1, ..PerQuery::default() },
            PerQuery { id: "b".into(), hits: 0, ..PerQuery::default() },
            PerQuery { id: "c".into(), hits: 1, ..PerQuery::default() },
            PerQuery { id: "d".into(), hits: 1, ..PerQuery::default() },
            PerQuery { id: "e".into(), hits: 1, ..PerQuery::default() },
        ];
        let result = mcnemar(&before, &after);
        assert_eq!((result.a_both_hit, result.b_before_only, result.c_after_only, result.d_both_miss), (1, 1, 3, 0));
        assert!((result.chi2 - 1.0).abs() < 1e-12);
        assert!(result.feasible);
        // chi2=1, df=1 -> p ~= 0.317; continuity-corrected chi2=0.25 -> p ~= 0.617.
        assert!((result.p_value - 0.3173).abs() < 1e-3, "p={}", result.p_value);
        assert!(!result.significant);
    }

    #[test]
    fn drift_threshold_flags_large_deltas_only() {
        let delta = DriftDelta { precision: 0.06, recall: 0.01, ..DriftDelta::default() };
        let (detected, reason) = drift_detected(&delta, 0.05);
        assert!(detected);
        assert!(reason.contains("ΔP="));
        let small = DriftDelta { precision: 0.01, recall: 0.01, ..DriftDelta::default() };
        assert!(!drift_detected(&small, 0.05).0);
    }
}

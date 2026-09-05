// Port of internal/runtime/trust_validation.go: memoized support-closure
// validation. Only approved claims with valid verification digests (and no
// dependency cycles) are trusted; everything else fails closed with a
// structured TrustValidationError.
use std::collections::HashMap;

use crate::claims::{
    is_claim_id, verify_claim_digest, Claim, ClaimError, CLAIM_STATUS_APPROVED,
};

#[derive(Debug, Clone, PartialEq, Eq)]
pub struct TrustValidationError {
    pub root_id: String,
    pub path: Vec<String>,
    pub reason: String,
}

impl std::fmt::Display for TrustValidationError {
    fn fmt(&self, f: &mut std::fmt::Formatter<'_>) -> std::fmt::Result {
        write!(
            f,
            "trust dependency validation failed for {}: {} (path: {})",
            self.root_id,
            self.reason,
            self.path.join(" -> ")
        )
    }
}

impl std::error::Error for TrustValidationError {}

#[derive(Debug, Clone, Copy, PartialEq, Eq)]
enum VisitState {
    Unvisited,
    Visiting,
    Valid,
    Invalid,
}

#[derive(Debug, Clone)]
struct TrustFailure {
    path: Vec<String>,
    reason: String,
}

type ClaimLoader = Box<dyn Fn(&str) -> Result<Claim, ClaimError>>;

pub struct TrustValidator {
    claims: HashMap<String, Claim>,
    load: Option<ClaimLoader>,
    states: HashMap<String, VisitState>,
    failures: HashMap<String, TrustFailure>,
    visit_count: HashMap<String, usize>,
    active_roots: HashMap<String, bool>,
}

impl TrustValidator {
    pub fn new(claims: Vec<Claim>) -> Result<Self, ClaimError> {
        let mut validator = Self {
            claims: HashMap::with_capacity(claims.len()),
            load: None,
            states: HashMap::with_capacity(claims.len()),
            failures: HashMap::new(),
            visit_count: HashMap::with_capacity(claims.len()),
            active_roots: HashMap::new(),
        };
        let mut ordered = claims;
        ordered.sort_by(|left, right| left.id.cmp(&right.id));
        for claim in ordered {
            if !is_claim_id(&claim.id) {
                return Err(crate::claims::message(format!(
                    "trust validator claim id {:?} is unsafe",
                    claim.id
                )));
            }
            if validator.claims.contains_key(&claim.id) {
                return Err(crate::claims::message(format!(
                    "trust validator claim {:?} is duplicated",
                    claim.id
                )));
            }
            validator.claims.insert(claim.id.clone(), claim);
        }
        Ok(validator)
    }

    pub fn from_store(
        store: &crate::claims::ClaimStore,
        workspace: &str,
    ) -> Result<Self, ClaimError> {
        crate::boundary::validate_workspace(&store.paths, workspace)?;
        let mut validator = Self::new(Vec::new())?;
        let store = store.paths.clone();
        let workspace = workspace.to_string();
        validator.load = Some(Box::new(move |id: &str| {
            crate::claims::ClaimStore::new(store.clone()).read(&workspace, id)
        }));
        Ok(validator)
    }

    pub fn visit_count(&self, id: &str) -> usize {
        self.visit_count.get(id).copied().unwrap_or(0)
    }

    pub fn validate_claim(&mut self, root: &Claim) -> Result<(), TrustValidationError> {
        let mut no_support_validation = |_: &Claim| Ok::<(), String>(());
        self.validate_claim_inner(root, &mut no_support_validation)
    }

    /// Validates the root claim, applying `validate` to every supporting
    /// claim in the closure (extra trust rules such as evidence closure).
    pub fn validate_claim_with_support(
        &mut self,
        root: &Claim,
        validate: &mut dyn FnMut(&Claim) -> Result<(), String>,
    ) -> Result<(), TrustValidationError> {
        self.validate_claim_inner(root, validate)
    }

    fn validate_claim_inner(
        &mut self,
        root: &Claim,
        validate: &mut dyn FnMut(&Claim) -> Result<(), String>,
    ) -> Result<(), TrustValidationError> {
        if !is_claim_id(&root.id) {
            return Err(TrustValidationError {
                root_id: root.id.clone(),
                path: vec![root.id.clone()],
                reason: format!("root claim id {:?} is unsafe", root.id),
            });
        }
        self.active_roots.insert(root.id.clone(), true);
        let result = self.validate_root_supports(root, validate);
        self.active_roots.remove(&root.id);
        result
    }

    fn validate_root_supports(
        &mut self,
        root: &Claim,
        validate: &mut dyn FnMut(&Claim) -> Result<(), String>,
    ) -> Result<(), TrustValidationError> {
        let mut ids = root.supporting_claim_ids.clone();
        ids.sort();
        let mut seen = std::collections::HashSet::new();
        for id in &ids {
            let path = vec![root.id.clone(), id.clone()];
            if !is_claim_id(id) {
                return Err(TrustValidationError {
                    root_id: root.id.clone(),
                    path,
                    reason: format!("supporting claim id {id:?} is unsafe"),
                });
            }
            if !seen.insert(id.clone()) {
                return Err(TrustValidationError {
                    root_id: root.id.clone(),
                    path,
                    reason: format!("supporting claim id {id:?} is duplicated"),
                });
            }
            self.visit(&root.id, id, &path, validate)?;
        }
        Ok(())
    }

    fn visit(
        &mut self,
        root_id: &str,
        id: &str,
        path: &[String],
        validate: &mut dyn FnMut(&Claim) -> Result<(), String>,
    ) -> Result<(), TrustValidationError> {
        if self.active_roots.get(root_id).copied().unwrap_or(false) && id == root_id {
            return self.cycle(root_id, id, path);
        }
        match self.states.get(id).copied().unwrap_or(VisitState::Unvisited) {
            VisitState::Valid => return Ok(()),
            VisitState::Invalid => {
                let failure = self.failures.get(id).cloned().unwrap_or(TrustFailure {
                    path: vec![id.to_string()],
                    reason: "invalid".into(),
                });
                return Err(new_trust_validation_error(
                    root_id,
                    &combine_trust_path(path, &failure.path),
                    &failure.reason,
                ));
            }
            VisitState::Visiting => return self.cycle(root_id, id, path),
            VisitState::Unvisited => {}
        }

        let mut claim = self.claims.get(id).cloned();
        if claim.is_none() {
            if let Some(load) = self.load.as_ref() {
                match load(id) {
                    Ok(loaded) => {
                        self.claims.insert(id.to_string(), loaded.clone());
                        claim = Some(loaded);
                    }
                    Err(load_err) => {
                        let reason = if load_err.is_not_found() {
                            format!("missing supporting claim {id}")
                        } else {
                            format!("supporting claim {id} cannot be parsed: {load_err}")
                        };
                        self.mark_invalid(id, TrustFailure {
                            path: vec![id.to_string()],
                            reason: reason.clone(),
                        });
                        return Err(new_trust_validation_error(root_id, path, &reason));
                    }
                }
            }
        }
        let Some(claim) = claim else {
            let reason = format!("missing supporting claim {id}");
            self.mark_invalid(id, TrustFailure {
                path: vec![id.to_string()],
                reason: reason.clone(),
            });
            return Err(new_trust_validation_error(root_id, path, &reason));
        };
        self.states.insert(id.to_string(), VisitState::Visiting);
        *self.visit_count.entry(id.to_string()).or_insert(0) += 1;
        if claim.status != CLAIM_STATUS_APPROVED {
            let reason = format!(
                "supporting claim {id} is {}; only approved claims are trusted",
                claim.status
            );
            self.mark_invalid(id, TrustFailure {
                path: vec![id.to_string()],
                reason: reason.clone(),
            });
            return Err(new_trust_validation_error(root_id, path, &reason));
        }
        if let Err(err) = verify_claim_digest(&claim) {
            let reason = format!("supporting claim {id}: {err}");
            self.mark_invalid(id, TrustFailure {
                path: vec![id.to_string()],
                reason: reason.clone(),
            });
            return Err(new_trust_validation_error(root_id, path, &reason));
        }
        {
            if let Err(err) = validate(&claim) {
                let reason = format!("supporting claim {id}: {err}");
                self.mark_invalid(id, TrustFailure {
                    path: vec![id.to_string()],
                    reason: reason.clone(),
                });
                return Err(new_trust_validation_error(root_id, path, &reason));
            }
        }

        let mut ids = claim.supporting_claim_ids.clone();
        ids.sort();
        let mut seen = std::collections::HashSet::new();
        for support_id in &ids {
            let child_path: Vec<String> = {
                let mut child = path.to_vec();
                child.push(support_id.clone());
                child
            };
            if !is_claim_id(support_id) {
                let failure = TrustFailure {
                    path: vec![id.to_string(), support_id.clone()],
                    reason: format!("supporting claim id {support_id:?} is unsafe"),
                };
                self.mark_invalid(id, failure.clone());
                return Err(new_trust_validation_error(
                    root_id,
                    &combine_trust_path(path, &failure.path),
                    &failure.reason,
                ));
            }
            if !seen.insert(support_id.clone()) {
                let failure = TrustFailure {
                    path: vec![id.to_string(), support_id.clone()],
                    reason: format!("supporting claim id {support_id:?} is duplicated"),
                };
                self.mark_invalid(id, failure.clone());
                return Err(new_trust_validation_error(
                    root_id,
                    &combine_trust_path(path, &failure.path),
                    &failure.reason,
                ));
            }
            if let Err(err) = self.visit(root_id, support_id, &child_path, &mut *validate) {
                if self.states.get(id).copied() != Some(VisitState::Invalid) {
                    if let Some(failure) = self.failures.get(support_id).cloned() {
                        self.mark_invalid(id, TrustFailure {
                            path: {
                                let mut combined = vec![id.to_string()];
                                combined.extend(failure.path);
                                combined
                            },
                            reason: failure.reason,
                        });
                    } else {
                        self.mark_invalid(id, TrustFailure {
                            path: vec![id.to_string(), support_id.clone()],
                            reason: err.reason.clone(),
                        });
                    }
                }
                return Err(err);
            }
        }
        self.states.insert(id.to_string(), VisitState::Valid);
        Ok(())
    }

    fn cycle(
        &mut self,
        root_id: &str,
        id: &str,
        path: &[String],
    ) -> Result<(), TrustValidationError> {
        let start = path.iter().position(|candidate| candidate == id).unwrap_or(0);
        let cycle_path: Vec<String> = path[start..].to_vec();
        let nodes: Vec<String> = cycle_path[..cycle_path.len() - 1].to_vec();
        for index in 0..nodes.len() {
            let mut rotated: Vec<String> = nodes[index..].to_vec();
            rotated.extend_from_slice(&nodes[..index]);
            rotated.push(nodes[index].clone());
            self.mark_invalid(&nodes[index], TrustFailure {
                path: rotated,
                reason: "dependency cycle detected".into(),
            });
        }
        Err(new_trust_validation_error(
            root_id,
            &cycle_path,
            "dependency cycle detected",
        ))
    }

    fn mark_invalid(&mut self, id: &str, failure: TrustFailure) {
        if self.states.get(id).copied() == Some(VisitState::Invalid) {
            return;
        }
        self.states.insert(id.to_string(), VisitState::Invalid);
        self.failures.insert(id.to_string(), failure);
    }
}

fn new_trust_validation_error(
    root_id: &str,
    path: &[String],
    reason: &str,
) -> TrustValidationError {
    TrustValidationError {
        root_id: root_id.to_string(),
        path: path.to_vec(),
        reason: reason.to_string(),
    }
}

fn combine_trust_path(current: &[String], suffix: &[String]) -> Vec<String> {
    if current.is_empty() {
        return suffix.to_vec();
    }
    let mut path = current[..current.len() - 1].to_vec();
    path.extend_from_slice(suffix);
    path
}

// ---------------------------------------------------------------------------
// Tests (port of trust_validation_test.go).
// ---------------------------------------------------------------------------

#[cfg(test)]
mod tests {
    use super::*;
    use crate::claims::{
        claim_verification_digest, CLAIM_BASIS_DERIVED, CLAIM_STATUS_DRAFT,
        CLAIM_STATUS_REVOKED, CLAIM_STATUS_SUPERSEDED,
    };

    fn trust_test_claim_id(number: u32) -> String {
        format!("clm_{number:032x}")
    }

    fn trust_test_claim(number: u32, status: &str, supporting: &[u32]) -> Claim {
        Claim {
            claim_type: crate::claims::OKF_CLAIM_TYPE.into(),
            id: trust_test_claim_id(number),
            tier: "projects".into(),
            status: status.into(),
            title: format!("claim {number}"),
            basis: CLAIM_BASIS_DERIVED.into(),
            created_at: "2026-01-01T00:00:00Z".into(),
            created_by: "test".into(),
            supporting_claim_ids: supporting
                .iter()
                .map(|n| trust_test_claim_id(*n))
                .collect(),
            body: format!("claim body {number}\n"),
            ..Claim::default()
        }
    }

    fn trust_test_approved_claim(number: u32, supporting: &[u32]) -> Claim {
        let mut claim = trust_test_claim(number, CLAIM_STATUS_APPROVED, supporting);
        claim.verified_at = "2026-01-01T00:00:00Z".into();
        claim.verified_by = "test".into();
        let digest = claim_verification_digest(&claim)
            .unwrap_or_else(|err| panic!("trust test claim digest: {err}"));
        claim.verified_digest = digest;
        claim
    }

    fn require_trust_validation_error(
        result: Result<(), TrustValidationError>,
    ) -> TrustValidationError {
        result.expect_err("ValidateClaim() error = nil")
    }

    #[test]
    fn trust_validation_chain() {
        let leaf = trust_test_approved_claim(3, &[]);
        let middle = trust_test_approved_claim(2, &[3]);
        let root = trust_test_approved_claim(1, &[2]);
        let mut validator = TrustValidator::new(vec![root.clone(), leaf, middle]).unwrap();
        validator.validate_claim(&root).unwrap();

        let mut deep = Vec::with_capacity(129);
        for number in (1..=129).rev() {
            if number == 129 {
                deep.push(trust_test_approved_claim(number, &[]));
            } else {
                deep.push(trust_test_approved_claim(number, &[number + 1]));
            }
        }
        let root = deep.last().unwrap().clone();
        let mut deep_validator = TrustValidator::new(deep).unwrap();
        deep_validator.validate_claim(&root).unwrap();
    }

    #[test]
    fn trust_validation_missing() {
        // missing node
        let root = trust_test_approved_claim(1, &[99]);
        let mut validator = TrustValidator::new(vec![root.clone()]).unwrap();
        let validation_err = require_trust_validation_error(validator.validate_claim(&root));
        assert!(
            validation_err.reason.contains("missing supporting claim"),
            "{}",
            validation_err.reason
        );
        assert_eq!(
            validation_err.path,
            vec![root.id.clone(), trust_test_claim_id(99)]
        );

        // unsafe node
        let root = trust_test_claim(1, CLAIM_STATUS_APPROVED, &[]);
        let mut root_unsafe = root.clone();
        root_unsafe.supporting_claim_ids = vec!["../outside".into()];
        let mut validator = TrustValidator::new(vec![root_unsafe.clone()]).unwrap();
        let validation_err = require_trust_validation_error(validator.validate_claim(&root_unsafe));
        assert!(validation_err.reason.contains("unsafe"), "{}", validation_err.reason);

        // duplicate node
        let support = trust_test_approved_claim(2, &[]);
        let mut root = trust_test_approved_claim(1, &[]);
        root.supporting_claim_ids = vec![support.id.clone(), support.id.clone()];
        let mut validator = TrustValidator::new(vec![root.clone(), support]).unwrap();
        let validation_err = require_trust_validation_error(validator.validate_claim(&root));
        assert!(
            validation_err.reason.contains("duplicated"),
            "{}",
            validation_err.reason
        );
    }

    #[test]
    fn trust_validation_status() {
        for (number, status) in [
            (2, CLAIM_STATUS_DRAFT),
            (3, CLAIM_STATUS_REVOKED),
            (4, CLAIM_STATUS_SUPERSEDED),
        ] {
            let support = trust_test_claim(number, status, &[]);
            let root = trust_test_approved_claim(1, &[number]);
            let mut validator = TrustValidator::new(vec![root.clone(), support]).unwrap();
            let validation_err = require_trust_validation_error(validator.validate_claim(&root));
            assert!(
                validation_err.reason.contains(status),
                "{status}: {}",
                validation_err.reason
            );
        }
    }

    #[test]
    fn trust_validation_digest() {
        let mut support = trust_test_approved_claim(2, &[]);
        support.body = "tampered\n".into();
        let root = trust_test_approved_claim(1, &[2]);
        let mut validator = TrustValidator::new(vec![root.clone(), support]).unwrap();
        let validation_err = require_trust_validation_error(validator.validate_claim(&root));
        assert!(
            validation_err.reason.contains("verification digest mismatch"),
            "{}",
            validation_err.reason
        );
    }

    #[test]
    fn trust_validation_cycle() {
        let first = trust_test_approved_claim(1, &[2]);
        let second = trust_test_approved_claim(2, &[1]);
        let mut validator = TrustValidator::new(vec![second, first.clone()]).unwrap();
        let validation_err = require_trust_validation_error(validator.validate_claim(&first));
        assert_eq!(validation_err.reason, "dependency cycle detected");
        assert_eq!(
            validation_err.path,
            vec![first.id.clone(), trust_test_claim_id(2), first.id.clone()]
        );
    }

    #[test]
    fn trust_validation_deterministic() {
        let missing_first = trust_test_claim_id(10);
        let missing_second = trust_test_claim_id(11);
        let mut root_a = trust_test_approved_claim(1, &[]);
        root_a.supporting_claim_ids = vec![missing_second.clone(), missing_first.clone()];
        let mut root_b = trust_test_approved_claim(1, &[]);
        root_b.supporting_claim_ids = vec![missing_first, missing_second];

        let mut first_validator = TrustValidator::new(vec![root_a.clone()]).unwrap();
        let mut second_validator = TrustValidator::new(vec![root_b.clone()]).unwrap();
        let first_err = require_trust_validation_error(first_validator.validate_claim(&root_a));
        let second_err = require_trust_validation_error(second_validator.validate_claim(&root_b));
        assert_eq!(first_err.to_string(), second_err.to_string());
    }

    #[test]
    fn trust_validation_memoized() {
        let shared = trust_test_approved_claim(3, &[]);
        let first = trust_test_approved_claim(1, &[3]);
        let second = trust_test_approved_claim(2, &[3]);
        let mut validator = TrustValidator::new(vec![second, shared.clone(), first]).unwrap();
        let first = trust_test_approved_claim(1, &[3]);
        let second = trust_test_approved_claim(2, &[3]);
        validator.validate_claim(&first).unwrap();
        validator.validate_claim(&second).unwrap();
        assert_eq!(validator.visit_count(&shared.id), 1);
    }
}

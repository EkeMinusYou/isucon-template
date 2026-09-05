# Objective / Constraint / Intervention workflow

## Authority

This document is the source of truth for the three-layer model, state transitions, READY and adoption conditions, priority, and writer rules. See [README.md](README.md) for CLI usage and storage details, and [Evidence](../../.agents/skills/_shared/evidence.md) for selecting and comparing evidence. Skill-specific operating limits stay in the responsible skill.

Use the backlog through `task backlog -- ...`. Do not edit `backlog.sqlite3` or `backlog.sql` manually. Every write includes an actor, a reason, and the target entity version where applicable.

Official behavior and validity come from `docs/official/`. Evidence is a basis for decisions, not a card kind; all B-xxx/B-xxxx cards are Interventions.

## Objective

Objective compares multiple Interventions against a stable final-result criterion.

```text
status: ACTIVE | RETIRED
mode: SATISFY | MAXIMIZE | MINIMIZE
```

An Objective has a metric or predicate, verification method, official sources, and optional parent. `required_for_valid_result` marks pass/fail requirements. MAXIMIZE and MINIMIZE Objectives do not become “resolved”; retire them only when the criterion no longer applies.

At the start, inspect ACTIVE Objectives with `task backlog -- objective list`. Initial template Objectives are:

- O-001: pass benchmark and final consistency checks
- O-002: satisfy the official restart persistence and reproducibility requirements
- O-003: maximize a valid benchmark score

Add contest-specific score components and penalties as Objectives after reading the official rules.

## Constraint

A Constraint is a solution-independent fact currently limiting one or more ACTIVE Objectives. Create one only when it has:

1. an ACTIVE Objective relation;
2. current Evidence with unit and denominator where numeric;
3. a stable identity and fingerprint;
4. snapshot or premise;
5. a causal path to the Objective;
6. a resolution or reconsider condition.

```text
status: ACTIVE | RESOLVED | INVALIDATED | MERGED
```

No candidate is required to keep a still-true Constraint ACTIVE. Lack of a candidate never blocks unrelated Intervention work. Record searched families and reconsider conditions in History.

Use RESOLVED when the observed fact no longer holds, INVALIDATED when its attribution was wrong, and MERGED for duplicates with the same identity and snapshot. Do not infer resolution from Intervention adoption alone; check the current resolution condition. Keep controllability and search progress out of status.

Terminal Constraints are immutable. A recurrence gets a versioned fingerprint. Merge duplicates into one ACTIVE survivor.

## Intervention

Every B-xxx or B-xxxx card is an Intervention. IDs use at least three digits and support four digits. An Intervention is one coherent adoption, application, and rollback boundary. File count, service count, implementation stages, and team size do not require splitting. Split only when the parts can be independently adopted and rolled back while retaining meaningful behavior.

The B-xxx/B-xxxx card ID is the stable Intervention identity. There is no separately named Intervention fingerprint: semantic duplicates are identified during investigation from their target, mechanism, and Change boundary rather than by equality of an arbitrary label.

```text
INVESTIGATE -> READY -> DOING -> VERIFY -> APPLIED -> VALIDATED
INVESTIGATE -> BLOCKED | REJECTED
READY -> INVESTIGATE | BLOCKED | REJECTED
DOING -> VERIFY | INVESTIGATE | BLOCKED | REJECTED
VERIFY -> DOING | APPLIED | INVESTIGATE | BLOCKED | REJECTED
APPLIED -> DOING | INVESTIGATE | BLOCKED | VALIDATED | REJECTED
BLOCKED -> INVESTIGATE | DOING | VERIFY | APPLIED
```

Terminal states do not reopen. `READY -> DOING` must set a non-empty Owner in the same update; READY has no Owner.

### READY gate

The card body must make four decisions clear without a separate generic contract schema:

1. `Hypothesis` — Objective and causal direction
2. `Change boundary` — implementation and rollback unit
3. `Verification` — observations and adoption/correction/rejection outcomes
4. `Safety` — official guardrails, stop condition, rollback

New cards always start in INVESTIGATE. Link at least one ACTIVE Objective and satisfy BLOCKING dependencies before READY; ORDERING dependencies need not be complete. `isucon-investigate` is the only skill that creates READY and confirms the four sections in one `resolve` operation.

The skill judges causal direction, a coherent application/rollback boundary, observable outcomes using standard Evidence or correctness checks, and official guardrails. The CLI checks structure rather than the quality of that reasoning; see [CLI checks](README.md#cli-checks).

Absence of a current Constraint or a direct metric does not by itself prevent READY. Effect magnitude may be unknown. A non-bottleneck optimization, selection change, loss recovery, spam control, or experiment may be READY when direction and safety are explainable. “Try it and inspect score” is insufficient.

### BLOCKED

Use BLOCKED only for a concrete external fact, permission, environment state, or unavailable artifact that current repository Evidence cannot provide. Record the question, inspected sources, established facts, minimum missing information, acquisition route, evidence baseline, resume trigger, and resume state in the card body or History. BLOCKED has no dedicated JSON contract or storage field.

Missing confidence, incomplete investigation, unknown effect size, or desire for a new measurement is not BLOCKED.

### VALIDATED and REJECTED

Use top-level `task pass` after a successful manual benchmark. It requires a finalized RUN with `passed=true` and a known score, and validates only APPLIED cards present in that RUN's before-bench snapshot with an unchanged Change boundary declaration. When a control RUN is declared, the final comparison must remain `compatible` and the outcome delta uses that control rather than the immediately preceding RUN.

Use `task pass FORCE=true` only for an explicit exceptional adoption. It bypasses the pass, known-score, and comparison-compatibility gates, but never finalization, snapshot integrity, or the Change boundary declaration check. The forced decision remains visible in History.

Adoption events are the source of truth for adoption decisions; `run.json` remains the source of truth for the RUN. See [Adoption records](README.md#adoption-records) for persistence and derived outputs.

During investigation, REJECTED requires evidence that the proposal is already resolved, violates official rules, lacks an explainable causal direction or safe boundary, duplicates an existing Intervention, or is technically refuted. Effort and size alone are not rejection reasons.

Do not reject or rollback from a single score fluctuation alone. Correctness failure, official-spec violation, operational failure, or evidence that refutes the causal path can justify rollback and REJECTED. Record the relevant RUN, mechanism evidence, and rollback result.

After a benchmark, prioritize a limited correction within the same Change boundary. If the problem cannot be corrected safely within that boundary, or Evidence refutes the improvement hypothesis, confirm that the finding is attributable to the target Intervention, then roll back and record REJECTED. Mechanism degradation and a correctness violation need not both be present.

## Relations

```text
Objective --constrained by--> Constraint
Objective --advanced by-----> Intervention
Constraint --RESOLVES-------> Intervention
Constraint --MITIGATES------> Intervention
Intervention --depends on---> Intervention
```

An Intervention does not need a Constraint relation. It should have an Objective relation before READY.

`RESOLVES` means the Intervention, alone or as a coherent dependency chain, is expected to satisfy the resolution condition. `MITIGATES` means positive but not independently resolving.

For performance RESOLVES, the skill supplies a structured residual assessment using the [CLI format](README.md#residual-assessment). Non-performance RESOLVES uses the Constraint evidence, resolution condition, and relation rationale; MITIGATES does not require the calculation.

Dependencies are explicit `ORDERING` or `BLOCKING` relations and always specify `required-status`. Change-boundary overlap and short-lived implementation order are not persisted dependencies.

## Evidence policy

Measurement is not a Backlog layer or card kind. Use existing standard RUN artifacts and code/spec evidence. If direction and safety are already explainable, do not require extra measurement before READY.

When Evidence is insufficient, record the limitation in Constraint or Intervention History. Do not create a measurement card. If a new standard instrumentation capability is truly required, treat it as a separately authorized repository task, not an implicit backlog transition.

Use the [Evidence policy](../../.agents/skills/_shared/evidence.md) for RUN selection, comparison, missing artifacts, and causal reasoning. Command behavior and snapshot hash semantics are documented in [README.md](README.md#evidence-and-snapshots).

## Priority

1. existing DOING, APPLIED awaiting verification, required rollback;
2. required-for-valid-result Objectives;
3. explicit user priority;
4. Interventions that RESOLVE ACTIVE Constraints and their unmet dependencies;
5. direct selection, value, and loss-recovery Interventions;
6. MITIGATES and ordinary positive Interventions.

Owner, dependency, dirty-diff safety, and deployment snapshot coherence take precedence over candidate ranking. Within a class, use Priority and the completed snapshot. The APPLIED work-in-progress limit is an operational rule enforced by [isucon-worker](../../.agents/skills/isucon-worker/SKILL.md), not by the CLI.

## Writer protocol

- Read current entity version immediately before writing.
- Include `--actor` and `--reason` on every mutation.
- Use `--expect-card-version`, `--expect-constraint-version`, or `--expect-objective-version`.
- On conflict, reread and merge intentionally; do not overwrite.
- Keep History append-only.
- Finish with `task backlog -- validate`.

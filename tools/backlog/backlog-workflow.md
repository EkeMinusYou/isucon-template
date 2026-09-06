# Objective / Constraint / Intervention workflow

## Authority

This is the source of truth for Backlog rules. Read only the sections required by the active skill. CLI usage and storage are in [README.md](README.md); evidence selection is in [Evidence](../../.agents/skills/_shared/evidence.md). Skill-specific limits stay in the responsible skill.

Use `task backlog -- ...` and the Writer protocol; never edit SQLite or its SQL dump manually. Official behavior and validity come from `docs/official/`.

## Objective

An Objective is a continuing final-result criterion for comparing Interventions.

```text
status: ACTIVE | RETIRED
mode: SATISFY | MAXIMIZE | MINIMIZE
```

Fields: metric/predicate, verification, official sources, optional parent. `required_for_valid_result` marks validity requirements. MAXIMIZE/MINIMIZE remain ACTIVE until their criterion no longer applies; then retire them.

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

Keep a still-true Constraint ACTIVE even without candidates; record searched families and reconsider conditions in History, and continue unrelated work. Status does not encode controllability or search progress.

Use RESOLVED only when the current resolution condition holds, not from Intervention adoption alone; INVALIDATED for wrong attribution; MERGED for duplicate identity/snapshot, keeping one ACTIVE survivor. Terminal Constraints are immutable; recurrence gets a versioned fingerprint.

## Intervention

An Intervention is one adoption, application, and rollback boundary. Its B-xxx/B-xxxx ID (at least three digits) is its identity; there is no separate fingerprint. Detect duplicates by target, mechanism, and Change boundary.

Split only when parts retain meaningful behavior and can be independently adopted and rolled back, not by file/service count, stages, or team size.

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

The card body must make three decisions clear without a separate generic contract schema:

1. `Hypothesis` — Objective and causal direction
2. `Change boundary` — implementation and rollback unit
3. `Verification` — observations and adoption/correction/rejection outcomes

New cards always start in INVESTIGATE. Link at least one ACTIVE Objective and satisfy BLOCKING dependencies before READY; ORDERING dependencies need not be complete. `isucon-investigate` is the only skill that creates READY and confirms the three sections in one `resolve` operation.

The skill judges these decisions using standard Evidence or correctness checks; the CLI validates structure only (see [CLI checks](README.md#cli-checks)). Unknown effect size, no current Constraint, or no direct metric does not prevent READY when causal direction and verification are explainable. This includes non-bottleneck improvements, selection, loss recovery, spam control, and experiments. “Try it and inspect score” is insufficient.

### BLOCKED

BLOCKED requires a concrete external fact, permission, environment state, or artifact unavailable from repository Evidence. In the body or History, record the question, inspected sources, facts, minimum missing information, acquisition route, evidence baseline, resume trigger, and resume state. No dedicated JSON contract is needed.

Low confidence, incomplete investigation, unknown effect size, and desire for measurement are not BLOCKED.

### Adoption

Use `task pass` to promote only APPLIED cards from the target RUN's before-bench snapshot to VALIDATED. Require:

- finalized RUN, `passed=true`, known score;
- usable snapshot and unchanged Change boundary declaration;
- `comparison.status=compatible` if a control RUN was declared; record delta against that control, not the preceding TSV row.

`task pass FORCE=true` is an explicit, reasoned exception recorded in History. It bypasses only pass, known-score, and comparison-compatibility checks, never finalization, snapshot integrity, or Change boundary matching.

Adoption events are authoritative for adoption; `run.json` for the RUN. Persistence and derived outputs: [Adoption records](README.md#adoption-records).

### Rejection during investigation

REJECTED requires Evidence of an already resolved proposal, official-spec violation, unexplainable causal direction, duplicate Intervention, or technical refutation. Effort, size, or a single RUN's score alone are insufficient.

### Rejection after application

Prefer a limited correction within the same Change boundary. If a concrete technical issue prevents correction within official rules and explicit requirements, or Evidence refutes the improvement hypothesis, confirm attribution to the target Intervention, roll back, and record REJECTED with the RUN, mechanism evidence, and rollback result.

Correctness failure, official-spec violation, operational failure, or causal refutation can justify rejection; mechanism degradation and correctness violation need not both exist. A single score fluctuation alone never justifies rejection or rollback.

## Relations

Objectives connect to Constraints and Interventions; Constraints are optional for Interventions. Objective links required for READY are defined in READY gate.

`RESOLVES` means the Intervention, alone or as a coherent dependency chain, is expected to satisfy the resolution condition. `MITIGATES` means positive but not independently resolving.

For performance RESOLVES, the skill supplies a structured residual assessment using the [CLI format](README.md#residual-assessment). Non-performance RESOLVES uses the Constraint evidence, resolution condition, and relation rationale; MITIGATES does not require the calculation.

Dependencies are explicit `ORDERING` or `BLOCKING` relations and always specify `required-status`. Change-boundary overlap and short-lived implementation order are not persisted dependencies.

## Evidence policy

RUNs, measurements, logs, profiles, code, configuration, and official sources are Evidence, not card kinds. Use existing Evidence; record gaps in Constraint or Intervention History, not measurement-only cards. Do not require extra measurement when READY conditions are already explainable. New or changed standard instrumentation requires a separately authorized repository task.

Selection and comparison: [Evidence](../../.agents/skills/_shared/evidence.md). CLI behavior and snapshot hashes: [README.md](README.md#evidence-and-snapshots).

## Priority

1. existing DOING, APPLIED awaiting verification, required rollback;
2. required-for-valid-result Objectives;
3. explicit user priority;
4. Interventions that RESOLVE ACTIVE Constraints and their unmet dependencies;
5. direct selection, value, and loss-recovery Interventions;
6. MITIGATES and ordinary positive Interventions.

Owner, dependency, conflicts with existing uncommitted changes, and deployment snapshot coherence take precedence over candidate ranking. Within a class, use Priority and the completed snapshot. The APPLIED work-in-progress limit is an operational rule enforced by [isucon-worker](../../.agents/skills/isucon-worker/SKILL.md), not by the CLI.

## Writer protocol

- Read current entity version immediately before writing.
- Include `--actor` and `--reason` on every mutation.
- Use `--expect-card-version`, `--expect-constraint-version`, or `--expect-objective-version`.
- On conflict, reread and merge intentionally; do not overwrite.
- Keep History append-only.
- Finish with `task backlog -- validate`.

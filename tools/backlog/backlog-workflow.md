# Objective / Constraint / Intervention workflow

## Authority

Use the backlog through `task backlog -- ...`. Do not edit `backlog.sqlite3` or `backlog.sql` manually. Every write includes an actor, a reason, and the target entity version where applicable.

Official behavior and validity come from `docs/official/`. Code, configuration, schema, saved RUNs, logs, profiles, and read-only runtime checks are Evidence.

## Objective

Objective compares multiple Interventions against a stable final-result criterion.

```text
status: ACTIVE | RETIRED
mode: SATISFY | MAXIMIZE | MINIMIZE
```

An Objective has a metric or predicate, verification method, official sources, and optional parent. `required_for_valid_result` marks pass/fail requirements. MAXIMIZE and MINIMIZE Objectives do not become “resolved”; retire them only when the criterion no longer applies.

```shell
task backlog -- objective list
task backlog -- objective show O-003
task backlog -- objective add --mode MAXIMIZE --title "..." \
  --metric-or-predicate "..." --verification "..." --actor human:name --reason "..."
task backlog -- objective update O-004 --expect-objective-version 0 \
  --status RETIRED --actor human:name --reason "..."
```

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

New cards always start in INVESTIGATE. READY is a deliberately narrow structural gate: the CLI requires those four non-empty sections, at least one ACTIVE Objective relation, and satisfied BLOCKING dependencies. It does not grade wording, require an effect estimate, or require ORDERING dependencies to be complete.

Effect magnitude may be unknown. A non-bottleneck optimization, selection change, loss recovery, spam control, or experiment may be READY when direction and safety are explainable. “Try it and inspect score” is insufficient.

### BLOCKED

Use BLOCKED only for a concrete external fact, permission, environment state, or unavailable artifact that current repository Evidence cannot provide. Record the question, inspected sources, established facts, minimum missing information, acquisition route, evidence baseline, resume trigger, and resume state in the card body or History. BLOCKED has no dedicated JSON contract or storage field.

Missing confidence, incomplete investigation, unknown effect size, or desire for a new measurement is not BLOCKED.

### VALIDATED and REJECTED

Use top-level `task pass` after a successful manual benchmark. It requires a finalized RUN with `passed=true` and a known score, and validates only APPLIED cards present in that RUN's before-bench snapshot with an unchanged Change boundary declaration. When a control RUN is declared, the final comparison must remain `compatible` and the outcome delta uses that control rather than the immediately preceding RUN.

Use `task pass FORCE=true` only for an explicit exceptional adoption. It bypasses the pass, known-score, and comparison-compatibility gates, but never finalization, snapshot integrity, or the Change boundary declaration check. The forced decision remains visible in History.

Adoption decisions are stored as immutable SQLite events in the same transaction as the card transitions. Score and comparison values in an event are evidence snapshots read from the finalized `run.json`; the manifest remains the source of truth for the RUN itself. `runs/outcomes.tsv` is regenerated from these events.

Do not reject or rollback from a single score fluctuation alone. Correctness failure, official-spec violation, operational failure, or evidence that refutes the causal path can justify rollback and REJECTED. Record the relevant RUN, mechanism evidence, and rollback result.

## Relations

```text
Objective --constrained by--> Constraint
Objective --advanced by-----> Intervention
Constraint --RESOLVES-------> Intervention
Constraint --MITIGATES------> Intervention
Intervention --depends on---> Intervention
```

An Intervention does not need a Constraint relation. It should have an Objective relation before READY.

`RESOLVES` means the resolution condition is expected to be satisfied. For a performance Constraint, supply the version 1 residual assessment with one shared axis plus current value and snapshot, expected reduction, added cost, and threshold. The CLI derives the residual and result. Capacity and shifted-work detail remain in the evidence or estimate basis rather than becoming additional assessment fields. `MITIGATES` means positive but not independently resolving; do not force the same calculation onto non-performance facts.

Dependencies are explicit `ORDERING` or `BLOCKING` relations and always specify `required-status`. Change-boundary overlap and short-lived implementation order are not persisted dependencies.

## Evidence policy

Measurement is not a Backlog layer or card kind. Use existing standard RUN artifacts and code/spec evidence. If direction and safety are already explainable, do not require extra measurement before READY.

When Evidence is insufficient, record the limitation in Constraint or Intervention History. Do not create a measurement card. If a new standard instrumentation capability is truly required, treat it as a separately authorized repository task, not an implicit backlog transition.

`evidence` chooses the newest finalized RUN whose APPLIED snapshot contains the requested card, or the exact `--run` when supplied. Comparisons come only from the target manifest's declared `compatible` control RUN. APPLIED snapshot schema version 3 uses `change_boundary_hash` to reject a RUN comparison or adoption when the normalized Change boundary declaration changed. This is declaration-staleness detection, not proof that the deployed implementation matches the text or that two texts are semantically equivalent. `decision_hash` (`Hypothesis`, `Verification`, and `Safety`) is audit information and produces a warning when it changed. Workflow metadata, relations, observations, results, and History are outside both hashes.

## Priority

1. existing DOING, APPLIED awaiting verification, required rollback;
2. required-for-valid-result Objectives;
3. explicit user priority;
4. RESOLVES Interventions and unmet dependencies;
5. direct selection, value, and loss-recovery Interventions;
6. MITIGATES and ordinary positive Interventions.

Within a class, preserve Priority, dependencies, Owner, dirty-diff safety, and deployment snapshot coherence. The APPLIED work-in-progress limit is an operational rule enforced by [isucon-worker](../../.agents/skills/isucon-worker/SKILL.md), not by the CLI.

## Writer protocol

- Read current entity version immediately before writing.
- Include `--actor` and `--reason` on every mutation.
- Use `--expect-card-version`, `--expect-constraint-version`, or `--expect-objective-version`.
- On conflict, reread and merge intentionally; do not overwrite.
- Keep History append-only.
- Finish with `task backlog -- validate`.

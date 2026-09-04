# Backlog

The CLI-managed backlog is the source of truth for Objective, Constraint, and Intervention state. Markdown cards are not used. The tracked database representation is `backlog.sql`; `backlog.sqlite3` is reconstructed local state.

```shell
task backlog
task backlog -- objective list
task backlog -- objective show O-003
task backlog -- constraint list
task backlog -- constraint show A-006
task backlog -- intervention show B-003
task backlog -- list --constraint A-006
task backlog -- validate
```

## Model

- Objective is a continuing result criterion: `SATISFY`, `MAXIMIZE`, or `MINIMIZE`.
- Constraint is an observed fact currently limiting an ACTIVE Objective.
- Intervention is a B-xxx or B-xxxx card: one adoption, application, and rollback boundary. The card ID is its stable identity; Intervention has no separate Fingerprint. IDs use at least three digits and support four digits.
- Evidence includes official documents, code, configuration, RUNs, logs, and profiles. It is not a card kind.

There is no card `kind` and no measurement-card lifecycle. All cards are Interventions. Existing standard measurement infrastructure remains available as Evidence infrastructure.

Initial template Objectives are:

- O-001: pass benchmark and final consistency checks
- O-002: satisfy the official restart persistence and reproducibility requirements
- O-003: maximize a valid benchmark score

Add contest-specific score components and penalties as Objectives after reading the official rules.

## Relations

An Intervention can advance an Objective directly. A Constraint must constrain at least one ACTIVE Objective.

Constraint–Intervention relations use:

- `RESOLVES`: predicted to satisfy the resolution condition
- `MITIGATES`: positive effect that does not independently resolve the Constraint

For a performance `RESOLVES` relation, the investigate workflow supplies the structured residual assessment. Non-performance `RESOLVES` relations use the Constraint's evidence and resolution condition plus the relation rationale; `MITIGATES` does not require a resolution calculation.

```shell
task backlog -- objective link O-003 --intervention B-700 --rationale "increase valid throughput" \
  --expect-objective-version 0 --actor skill:isucon-analyze --reason "candidate advances score objective"

task backlog -- constraint add --objective O-003 --title "..." --fingerprint "..." \
  --scope "..." --evidence "..." --resolution "..." \
  --actor skill:isucon-analyze --reason "observed current constraint"

task backlog -- constraint link A-008 --card B-700 --role MITIGATES \
  --expect-constraint-version 0 --actor skill:isucon-investigate --reason "positive partial reduction"
```

The performance residual assessment stores only a shared axis, current value and snapshot, expected reduction, added cost, and threshold. The CLI derives the predicted residual and whether it resolves the Constraint. See [examples/constraint-assessment.json](examples/constraint-assessment.json).

## Intervention lifecycle

```text
INVESTIGATE -> READY -> DOING -> VERIFY -> APPLIED -> VALIDATED
INVESTIGATE -> BLOCKED | REJECTED
```

`isucon-investigate` is the only skill that creates READY. A READY Intervention states:

1. `Hypothesis`: Objective and causal direction
2. `Change boundary`: implementation and rollback unit
3. `Verification`: adoption, correction, and rejection criteria
4. `Safety`: official guardrails, stop condition, and rollback

Unknown effect magnitude, absence of a current Constraint, or lack of a direct metric does not by itself prevent READY. A proposal that can only say “change it and inspect score” remains INVESTIGATE.

New cards always start as INVESTIGATE. The CLI permits READY only when the four sections above are non-empty, an ACTIVE Objective is linked, and every BLOCKING dependency is satisfied. It deliberately does not grade prose, require a known effect size, or block READY on ORDERING dependencies. `READY -> DOING` sets Owner atomically. Targeted writes require `--expect-card-version`; Constraint and Objective writes use their own version checks. Dependencies always specify `required-status` and `mode` explicitly.

After a successful manual benchmark, use `task pass` or `task pass -- B-001,B-002`. The RUN must be finalized, have `passed=true`, and have a known score. If a control RUN was declared, its final comparison status must be `compatible`; the recorded delta uses that control rather than the previous TSV row. `task pass FORCE=true` overrides only the pass, known-score, and comparison-compatibility gates; finalization, a usable APPLIED snapshot, and an unchanged Change boundary declaration remain mandatory. Forced adoption is recorded in History.

Each pass writes one immutable `adoption_events` row and its `adoption_event_cards` rows in the same SQLite transaction as every card promotion. The event snapshots score, pass state, declared control, delta, manifest hash, and backlog revision from `run.json`; each card row retains its `change_boundary_hash`. `runs/outcomes.tsv` is an atomically regenerated projection, not a source of truth.

APPLIED snapshot schema version 3 stores two deliberately small hashes. `change_boundary_hash` contains only the normalized `Change boundary`; it detects whether that declaration changed between snapshot capture and comparison or adoption. It does not prove that deployed code matches the declaration or that two differently worded declarations are semantically equivalent. `decision_hash` contains `Hypothesis`, `Verification`, and `Safety`; changing it produces a review warning but does not make an unchanged Change boundary incompatible or block adoption. Title, priority, owner, run links, Objective/Constraint relations, Observation, Unknowns, Result, and History are outside both hashes. Line-ending/trailing-space changes and equivalent JSON formatting are normalized. Benchmark and Evidence commands accept only version 3 snapshots.

The performance residual assessment stores the same `change_boundary_hash`. Editing Hypothesis, Verification, or Safety does not force that calculation to be repeated; editing the declared Change boundary does.

To inspect evidence, the CLI selects the newest finalized RUN whose APPLIED snapshot actually contains the card. Use `--run` to select one explicitly. Endpoint, TSV, and profile comparisons use only the control RUN declared compatible by that target manifest; they never fall back to an unrelated previous RUN.

```shell
task backlog -- evidence B-001
task backlog -- evidence --run 20260904-120000 B-001
```

## Storage and validation

Every CLI mutation increments `backlog_revision` and dumps the database. Do not edit SQLite directly.

```shell
task backlog -- validate
```

Validation checks SQLite integrity, IDs and versions, card states, READY contracts, dependencies, Objective hierarchy, ACTIVE Constraint scope, Constraint relations, and assessment bindings.

See [backlog-workflow.md](backlog-workflow.md) for writer and state rules.

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

## CLI usage

The model, relation meanings, lifecycle, and decision rules are defined in [backlog-workflow.md](backlog-workflow.md). Read its [writer protocol](backlog-workflow.md#writer-protocol) before making changes.

```shell
task backlog -- objective add --mode MAXIMIZE --title "..." \
  --metric-or-predicate "..." --verification "..." --actor human:name --reason "..."
task backlog -- objective update O-004 --expect-objective-version 0 \
  --status RETIRED --actor human:name --reason "..."
```

```shell
task backlog -- objective link O-003 --intervention B-700 --rationale "increase valid throughput" \
  --expect-objective-version 0 --actor skill:isucon-analyze --reason "candidate advances score objective"

task backlog -- constraint add --objective O-003 --title "..." --fingerprint "..." \
  --scope "..." --evidence "..." --resolution "..." \
  --actor skill:isucon-analyze --reason "observed current constraint"

task backlog -- constraint link A-008 --card B-700 --role MITIGATES \
  --expect-constraint-version 0 --actor skill:isucon-investigate --reason "positive partial reduction"
```

## Residual assessment

Use `constraint assess` or `constraint link --assessment-json` with [examples/constraint-assessment.json](examples/constraint-assessment.json). The version 1 format stores a shared axis, current value and snapshot, expected reduction, added cost, and threshold. The CLI derives the predicted residual and whether it resolves the Constraint. Capacity and shifted-work detail remain in the evidence or estimate basis.

The skill decides when an assessment is required under the [relation rules](backlog-workflow.md#relations). The CLI validates supplied assessments; it does not classify a Constraint as performance-related from prose.

## CLI checks

The CLI enforces the [lifecycle and READY contract](backlog-workflow.md#intervention) structurally: the three sections (Hypothesis, Change boundary, Verification) must be non-empty, an ACTIVE Objective must be linked, and BLOCKING dependencies must be satisfied. It does not grade prose or require a known effect size. ORDERING dependencies do not block READY. `READY -> DOING` uses `update` with a non-empty Owner atomically; investigation uses `resolve` to commit the body and outcome together.

Targeted writes require `--expect-card-version`; Constraint and Objective writes use their own version checks. Dependencies explicitly supply `required-status` and `mode`. Skill-specific policies such as the worker's APPLIED limit are not CLI gates.

## Adoption records

Use `task pass` or `task pass -- B-001,B-002` after checking the [adoption conditions and FORCE exceptions](backlog-workflow.md#adoption).

Each pass writes one immutable `adoption_events` row and its `adoption_event_cards` rows in the same SQLite transaction as every card promotion. The event snapshots score, pass state, declared control, delta, manifest hash, and backlog revision from `run.json`; each card row retains its `change_boundary_hash`. `runs/outcomes.tsv` is an atomically regenerated projection, not a source of truth.

## Evidence and snapshots

Benchmark and Evidence commands accept only APPLIED snapshot schema version 3.

| Hash | Input | Effect of a change |
| --- | --- | --- |
| `change_boundary_hash` | Normalized `Change boundary` | Invalidates comparison/adoption against the old snapshot and requires reassessment of a bound performance residual |
| `decision_hash` | `Hypothesis`, `Verification` | Review warning only; does not invalidate comparison/adoption or require residual recalculation |

All other fields, including workflow metadata, relations, observations, results, and History, are excluded. Line endings, trailing spaces, and equivalent JSON formatting are normalized. These hashes detect stale declarations; they do not prove deployed-code agreement or semantic equivalence of differently worded declarations.

To inspect evidence, the CLI selects the newest finalized RUN whose APPLIED snapshot actually contains the card. Use `--run` to select one explicitly. Endpoint, TSV, and profile comparisons use only the control RUN declared compatible by that target manifest; they never fall back to an unrelated previous RUN.

```shell
task backlog -- evidence B-001
task backlog -- evidence --run 20260904-120000 B-001
```

## Storage and validation

Every CLI mutation increments `backlog_revision` and dumps the database. Use the [writer protocol](backlog-workflow.md#writer-protocol); do not edit SQLite or its SQL dump manually.

```shell
task backlog -- validate
```

Validation checks SQLite integrity, IDs and versions, card states, READY contracts, dependencies, Objective hierarchy, ACTIVE Constraint scope, Constraint relations, and assessment bindings.

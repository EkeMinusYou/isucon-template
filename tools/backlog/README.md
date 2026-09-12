# Backlog

The CLI-managed backlog is the source of truth for Objective, Target, and Intervention state. Markdown cards are not used. The tracked database representation is `backlog.sql`; `backlog.sqlite3` is reconstructed local state.

```shell
task backlog
task backlog -- objective list
task backlog -- objective show O-008
task backlog -- target list
task backlog -- target show A-006
task backlog -- intervention show B-003
task backlog -- list --target A-006
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
# Create a goal-bearing Target; the initial Objective is primary.
task backlog -- target add --objective O-012 --title "Reduce viewing-start latency" \
  --fingerprint "viewing-start|current-premise|v1" --scope "Search to viewing start" \
  --axis "scenario duration" --evidence "saved RUN and current code references" \
  --goal "Lower viewing-start scenario latency relative to the measured baseline" \
  --evaluation "Compare the same scenario, workload, and response semantics" \
  --actor skill:isucon-target --reason "Less sequential wait may advance scoring actions"

# Use a fitting Target when available; --target is optional (fill in the standard card body).
task backlog -- add --target A-008 --title "Batch related data reads" \
  --actor skill:isucon-analyze --reason "Reduce sequential round trips"

# Additional links are optional; --primary selects the main destination.
task backlog -- objective link O-012 --target A-008 --primary --rationale "Less scenario wait may advance scoring actions" \
  --expect-objective-version 0 --expect-target-version 0 \
  --actor skill:isucon-target --reason "Refine primary outcome"
task backlog -- target link A-008 --card B-700 --primary \
  --expect-target-version 0 --expect-card-version 0 \
  --actor skill:isucon-investigate --reason "This change improves target latency"

# Resolution records attained-goal evidence independently of adoption.
task backlog -- target transition A-008 --status RESOLVED --completion-evidence "RUN comparison demonstrating the goal" \
  --expect-target-version 1 --actor skill:isucon-target --reason "Goal attained under stated evaluation conditions"
```

Use actual IDs and current versions. `target update --axis ... --goal ... --evaluation ...` revises the effort; preserve the previous goal and reason in History. Recurrence uses `target add --previous-target A-ID` with a new fingerprint and the changed premise in `--reason`; the previous Target must be terminal. First links are primary automatically; additional links preserve that choice unless `--primary` is supplied. Each link set has exactly one primary destination.

## Automation without wrapper scripts

Use `--format json` to read structured data instead of parsing terminal text. Text output remains the default.

```shell
task --silent backlog -- show B-003 --format json
task --silent backlog -- list --status READY --unowned --format json
task --silent backlog -- target show A-006 --format json
task --silent backlog -- objective list --format json
```

`show` returns one entity, including `id`, `version`, and `status`. JSON keys use snake_case.
`list` returns an object with `backlog_revision`, `cards`, `targets`, and `objectives`, matching the groups
in the terminal view. Card filters apply to `cards`; `--all` also includes terminal Targets and retired
Objectives. Card list entries omit section bodies, History, and historical assessments; use `show` for
those details. `target list` and `objective list` return arrays. Empty top-level collections are `[]`;
empty nested relations may be `null`. `intervention show/list` support the same formats as `show/list`.
JSON output is incompatible with `watch` / `--watch`.

Intervention `add`, `update`, `resolve`, and `transition` also accept `--format json`. Their response is
`{"id":"B-003","version":8,"woke":[]}`. It reports the version committed by that operation; it does not
reread the card and accidentally return a version from a later writer. Creation returns version 0.
`woke` contains dependent card IDs automatically moved from BLOCKED to INVESTIGATE. Read those cards
before editing them. Check the command's exit status, including SQL dump failures, before using its response.

Use the version from the card you inspected in `--expect-card-version`. The JSON response provides the
next version for follow-up writes based on the same inspected content and your own changes. A concurrent
change still causes a conflict: reread and reconsider the update instead of automatically retrying with
the newest version. Actor, reason, ownership, and lifecycle requirements are unchanged.

```shell
# Claim the inspected READY card; version numbers below are examples.
task --silent backlog -- update B-003 --status DOING --owner agent:worker \
  --expect-card-version 7 --actor agent:worker --reason "Start implementation" --format json

# Record a result without constructing a JSON input document.
task --silent backlog -- update B-003 --result "Local correctness checks passed" \
  --expect-card-version 8 --actor agent:worker --reason "Record verification" --format json

# Save a multiline result and change status in one transaction.
task --silent backlog -- transition B-003 --status VERIFY --result-file verification.txt \
  --expect-card-version 9 --actor agent:worker --reason "Implementation verified" --format json
```

`--result TEXT` and `--result-file PATH` are available on all four Intervention write commands above.
Use `--result-file -` to read plain text from stdin. They replace the Result section; empty or whitespace-only
text removes it, and trailing newlines are trimmed as with `--section-stdin`. The operation's reason is
appended to History. To retain earlier observations in the current Result, include them in the replacement text.
`--section-stdin` remains available on `add`, `update`, and `resolve` for other sections. It can be combined
with a direct Result input only when its JSON object does not also contain Result and both inputs do not
consume stdin. Conflicting inputs and unreadable files fail before mutation. A failed status transition
does not save the supplied Result, increment the version, or append History.

## Goal assessment

Targets use an evaluation axis, baseline Evidence, a numeric or decidable goal, and evaluation conditions. `IMPROVES` is the common Intervention relation. No mandatory residual assessment or `RESOLVES`/`MITIGATES` classification is needed. Historical assessments remain audit records, not new-card requirements. Adoption does not automatically resolve a Target.

## CLI checks

The CLI enforces the [lifecycle and READY contract](backlog-workflow.md#intervention) structurally: the three sections (Hypothesis, Change boundary, Evaluation) must be non-empty and BLOCKING dependencies must be satisfied. Target links are optional. When linked, READY admission requires a primary ACTIVE Target with a primary ACTIVE Objective, and every non-empty link set has exactly one primary destination. It does not grade prose or require a known effect size. ORDERING dependencies do not block READY. `READY -> DOING` uses `update` with a non-empty Owner atomically; investigation uses `resolve` to commit the body and outcome together.

Targeted writes require `--expect-card-version`; Target and Objective writes use their own version checks. Primary-link changes also require the version of the entity owning the link set: `objective link --primary` requires `--expect-target-version`, and `target link --primary` requires `--expect-card-version`. Relation mutations increment both entity versions atomically; reread them before the next write. Dependencies explicitly supply `required-status` and `mode`. Skill-specific policies such as the worker's APPLIED limit are not CLI gates.

## Adoption records

Use `task pass` or `task pass -- B-001,B-002` after checking the [adoption conditions and FORCE exceptions](backlog-workflow.md#adoption).

Each pass writes one immutable `adoption_events` row and its `adoption_event_cards` rows in the same SQLite transaction as every card promotion. The event snapshots score, pass state, declared control, delta, manifest hash, and backlog revision from `run.json`; each card row retains its `change_boundary_hash`. `runs/outcomes.tsv` is an atomically regenerated projection, not a source of truth.

## Evidence and snapshots

Benchmark and Evidence commands accept only APPLIED snapshot schema version 3.

| Hash | Input | Effect of a change |
| --- | --- | --- |
| `change_boundary_hash` | Normalized `Change boundary` | Invalidates comparison/adoption against the old snapshot and makes any historical assessment bound to the old boundary stale |
| `decision_hash` | `Hypothesis`, `Evaluation` | Review warning only; does not invalidate comparison/adoption or require a quantified gain |

All other fields, including workflow metadata, relations, observations, results, and History, are excluded. Line endings, trailing spaces, and equivalent JSON formatting are normalized. These hashes detect stale declarations; they do not prove deployed-code agreement or semantic equivalence of differently worded declarations.

To inspect evidence, the CLI selects the newest finalized RUN whose APPLIED snapshot actually contains the card. Use `--run` to select one explicitly. Endpoint, TSV, and profile comparisons prefer the card's `Compare Run`, set with `--compare-run` on create/update. With multiple explicit RUNs, the latest specified RUN is used. If unset, evidence selects the latest earlier finalized RUN with identical roles (including entry and additional roles). An invalid explicit selection does not fall back. This selects evidence for mechanism review, not a verified causal baseline; check source, load windows, workload, and concurrent changes before attributing improvement. Adoption and score deltas still use the target manifest's declared control and its compatibility checks.

```shell
task backlog -- evidence B-001
task backlog -- evidence --run 20260904-120000 B-001
task backlog -- evidence --format json --run 20260904-120000 B-001,B-002
```

`Evaluation` can contain a version 1 JSON extraction contract or free text. The evidence command extracts the specified endpoint/profile/TSV references for JSON; free text is displayed as checks without automatic metric extraction. Neither form automatically decides adoption. See [Evaluation reference](../../.agents/skills/_shared/evaluation.md) for supported fields, lookup limits, and how to retain decision criteria. Define available references when preparing READY cards; existing free-text cards do not require bulk conversion.

## Storage and validation

Every CLI mutation increments `backlog_revision` and dumps the database. Use the [writer protocol](backlog-workflow.md#writer-protocol); do not edit SQLite or its SQL dump manually.

```shell
task backlog -- validate
```

Validation checks SQLite integrity, IDs and versions, card states, READY contracts, dependencies, Objective hierarchy, ACTIVE Target scope and goals, primary links, Target relations, and historical assessment bindings.

For existing cards, migrate the old section with `task backlog -- migrate-evaluation B-001 --expect-card-version N --actor human:name --reason "Rename and scope evaluation" < evaluation.txt`. The input is the revised Evaluation text. This one-time operation preserves status, Owner, section order, and the previous text in History, including for terminal cards. Normal writes accept only `Evaluation`.

## Model migration

Target migration preserves existing A-IDs, Evidence, History, adoption decisions, and legacy relation/assessment records. Terminal historical Interventions can carry an explicit legacy-link exception; this remains an audit accommodation. Current creation and READY permit unlinked Interventions while preserving the body and dependency gates. Objective organization and common validity conditions are documented in [Objective organization](objective-organization.md) and [Common validity conditions](common-validity.md).

A fresh template ledger contains no Objectives, Targets, Interventions, or adoption history. Establish Objectives from the current contest’s official rules and Evidence using `isucon-objective`. Opening the ledger does not seed contest-specific hypotheses.

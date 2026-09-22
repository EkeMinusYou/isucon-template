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
task backlog -- objective add --mode MAXIMIZE --title "..." --priority P1 \
  --metric-or-predicate "..." --verification "..." --actor human:name --reason "..."
task backlog -- objective update O-004 --expect-objective-version 0 \
  --priority P0 --actor human:name --reason "..."
```

```shell
# Create a goal-bearing Target; the initial Objective is primary.
task backlog -- target add --objective O-012 --title "Reduce viewing-start latency" \
  --fingerprint "viewing-start|current-premise|v1" --scope "Search to viewing start" \
  --axis "scenario duration" --evidence "saved RUN and current code references" \
  --goal "Lower viewing-start scenario latency relative to the measured baseline" \
  --evaluation "Compare the same scenario, workload, and response semantics" \
  --actor skill:isucon-target --reason "Less sequential wait may advance scoring actions"

# Use a fitting Target when available; --target is optional. New cards start in INVESTIGATE,
# so the Change boundary and other READY sections may be filled in later.
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
task --silent backlog -- show B-003 --format json --fields id,version,status,owner,sections
task --silent backlog -- list --status READY --unowned --format json
task --silent backlog -- list --status READY --format json --fields id,status,priority,owner,title
task --silent backlog -- target show A-006 --format json
task --silent backlog -- target show A-006 --format json --fields id,version,status,goal,evaluation
task --silent backlog -- objective show O-001 --format json --fields id,version,status,priority,metric_or_predicate
task --silent backlog -- objective list --status ACTIVE --priority P0 --format json
```

`show` returns one entity, including `id`, `version`, and `status`. JSON keys use snake_case.
`list` returns an object with `backlog_revision`, `cards`, `targets`, and `objectives`, matching the groups
in the terminal view. By default, `cards` excludes terminal Interventions (`VALIDATED`/`REJECTED`);
`--all` includes those cards and also includes terminal Targets and retired Objectives. Use the default
list for discovery, duplicate checks, and reuse of prior investigation. Reserve `--all` for explicit
lifecycle, migration, or audit work. Card list entries omit section bodies, History, and historical
assessments; use `show` for those details. `target list` and `objective list` return arrays. Empty
top-level collections are `[]`; empty nested relations may be `null`. `intervention show/list` support
the same formats as `show/list`.
JSON output is incompatible with `watch` / `--watch`.

For a compact Intervention list, add `--fields` with comma-separated card JSON keys.
This requires `--format json` and returns only `backlog_revision` and `cards`, with
each card containing exactly the selected fields. Targets and Objectives are not
loaded or included. Existing filters and ordering still apply; no matches returns
`cards: []`. Values retain their JSON types, including empty strings and null relations.
Whitespace around field names is ignored and duplicate names are deduplicated.
Empty or unknown names are rejected with the available field names. `sections`,
`history`, and `target_assessments` require `show` and cannot be selected here.
The `intervention list` alias also accepts `--fields`. Without this option, output
is unchanged.

`show`, `intervention show`, `target show`, and `objective show` also accept `--fields`
with `--format json`. They return one object containing exactly the selected top-level
JSON keys, without an envelope. All JSON fields of the entity are selectable, including
`sections`, `history`, and `target_assessments` where present in its model. Explicitly
selected empty fields remain present (including `null`), even if full output would omit
them. Types and array order are preserved. Whitespace and duplicate field names are
handled as in `list`; empty or unknown names fail with available fields for that entity.
Without `--fields`, existing text and JSON output are unchanged. This option limits
output; it does not change database loading or provide nested field filtering.

To read only selected sections and history entries not yet reviewed, filter the returned
JSON before displaying it (replace the position with the last reviewed position):

```shell
task --silent backlog -- show B-003 --format json --fields id,version,sections,history |
  jq '{id,version,sections: [.sections[]? | select(.name == "Change boundary" or .name == "Evaluation")], history: [.history[]? | select(.position > 4)]}'
```

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

## Implementation time estimates

Interventions have a nullable integer `implementation_estimate_minutes` field. It estimates, in whole minutes, the rough normal-path time for `isucon-worker` to claim the card, implement the approved boundary, run the required local checks and build, deploy it, and complete the worker's remote checks. Investigation, dependency/Owner waits, user-run manual benchmarks, and verifier adoption review are excluded. It is a single planning estimate, not a measured runtime or a promised completion time.

Set it with `--implementation-estimate-minutes 30` on `update` or `resolve`; new cards created by `add` intentionally leave it unset. Use `--implementation-estimate-minutes ''` on `update` or `resolve` to clear it. Omission preserves the current value on update/resolve. Zero, negative, fractional, and nonnumeric values are rejected atomically. The usual version and History requirements apply.

`isucon-investigate` estimates each investigated card once its implementation boundary is concrete, using the normal work path of `isucon-worker` rather than a conservative human-equivalent effort or an uncertainty allowance, and supplies one rough integer estimate in the same READY resolution as the final body and Priority. Do not add a range, uncertainty note, or safety padding to the estimate. Reassess after splitting, merging, or changing scope. An unestimable non-READY card stays unset with its reason and reassessment conditions in Unknowns. This is a skill requirement, not an additional CLI READY gate.

`task backlog` (including watch) displays `30m` or `-`. Card details include the estimate; JSON list/show expose a number or `null`, also selectable using `--fields id,implementation_estimate_minutes`. Existing databases migrate automatically with NULL estimates; existing card versions, History, and saved RUN snapshots are preserved. Estimates do not affect snapshot hashes or automatically change Priority.

Objectives have the same `priority` JSON/text field as Targets and Interventions. Use `--priority P0|P1|P2` on `objective add` or `objective update`; `objective list` accepts `--priority` and `-p` as filters. Priority updates use the normal objective version check and append the supplied reason to Objective History. Existing databases add the column automatically with an empty value; assign it when `isucon-objective` next creates or reassesses the Objective.

## Goal assessment

Targets use an evaluation axis, baseline Evidence, a numeric or decidable goal, and evaluation conditions. `IMPROVES` is the common Intervention relation. No mandatory residual assessment or `RESOLVES`/`MITIGATES` classification is needed. Historical assessments remain audit records, not new-card requirements. Adoption does not automatically resolve a Target.

## CLI checks

The CLI enforces the [lifecycle and READY contract](backlog-workflow.md#intervention) structurally when a card enters READY or a later state that requires the READY contract: the three sections (Hypothesis, Change boundary, Evaluation) must then be non-empty and BLOCKING dependencies must be satisfied. `add` always creates an INVESTIGATE card and does not require the READY contract to be complete; a Change boundary may be added later during investigation. Target links are optional. When linked, READY admission requires a primary ACTIVE Target with a primary ACTIVE Objective, and every non-empty link set has exactly one primary destination. It does not grade prose or require a known effect size. ORDERING dependencies do not block READY. `READY -> DOING` uses `update` with a non-empty Owner atomically; investigation uses `resolve` to commit the body and outcome together.

Targeted writes require `--expect-card-version`; Target and Objective writes use their own version checks. Primary-link changes also require the version of the entity owning the link set: `objective link --primary` requires `--expect-target-version`, and `target link --primary` requires `--expect-card-version`. Relation mutations increment both entity versions atomically; reread them before the next write. Dependencies explicitly supply `required-status` and `mode`. APPLIED has no count limit. Worktree and deployment exclusivity remain operational rules.

## Ownership and handoff

Use a unique Owner per execution. Claims and handoffs use existing status, Owner, version and append-only History. The following examples use successive versions; always reread the actual card rather than assuming these numbers.

```shell
# Worker publishes a checked application and releases it to verifier.
task backlog -- transition B-001 --status APPLIED --release-owner --expect-owner worker:session-1 --expect-card-version 3 --actor worker:session-1 --reason 'Deployed and checked batch; source and checks: ...'
# Verifier claims the application represented in the requested RUN.
task backlog -- update B-001 --owner verifier:session-1 --expect-owner '' --expect-card-version 4 --actor verifier:session-1 --reason 'Review RUN 20260901-120000, application B-001@4'
# Verifier atomically records the decision and returns correction work.
task backlog -- transition B-001 --status DOING --release-owner --expect-owner verifier:session-1 --expect-card-version 5 --actor verifier:session-1 --reason 'Rejection cleanup: RUN 20260901-120000, application B-001@4; evidence: ...; remove: ...; retain: ...; completion checks: ...'
# Worker claims unowned DOING without changing its state.
task backlog -- update B-001 --owner worker:session-2 --expect-owner '' --expect-card-version 6 --actor worker:session-2 --reason 'Take rejection cleanup before new READY'
# Worker publishes the cleanup application and waits for the next user benchmark.
task backlog -- transition B-001 --status APPLIED --release-owner --expect-owner worker:session-2 --expect-card-version 7 --actor worker:session-2 --reason 'rejection-cleanup application: removed ...; checks/deploy ...; next user benchmark required; verifier closes REJECTED'
# After that benchmark, verifier claims the cleanup application and closes it.
task backlog -- update B-001 --owner verifier:session-2 --expect-owner '' --expect-card-version 8 --actor verifier:session-2 --reason 'Review post-cleanup RUN 20260901-130000, rejection-cleanup application B-001@8'
task backlog -- transition B-001 --status REJECTED --release-owner --expect-owner verifier:session-2 --expect-card-version 9 --actor verifier:session-2 --reason 'REJECTED after post-cleanup benchmark RUN 20260901-130000; cleanup conditions satisfied'
```

`--release-owner` requires `--expect-owner` and is supported for APPLIED → DOING, application and rejection. The transition, Owner release, optional Result, and reason in History commit together. A stale version or unexpected Owner leaves all of them unchanged. `transition --status VALIDATED` is rejected; adoption must use `task pass`. A second claimant cannot overwrite the first with the empty-Owner precondition. Existing APPLIED owners release explicitly with a version/Owner-checked `update --owner ''`; verifier never steals them.

Worker returns limited corrections and rejection cleanup to APPLIED after checks/deploy, generating a fresh application ID. A rejection-cleanup application is not adopted; after the next user benchmark, verifier reviews it and closes it as REJECTED. The CLI keeps dependency checks; coordinate affected dependent cards rather than forcing a transition that invalidates their contracts. See [workflow](backlog-workflow.md#ownership-and-handoff) for authority and shared-environment exclusivity.

## Adoption records

Use `task pass RUN=20260901-120000 OWNER=verifier:session-1 VERSIONS=B-001=4,B-002=7 -- B-001,B-002` after checking the [adoption conditions and FORCE exceptions](backlog-workflow.md#adoption).

This command is for ordinary APPLIED applications, including applications after a limited correction. Do not use it for an APPLIED card whose History identifies the application as `rejection-cleanup application`; verifier reviews the post-cleanup RUN and transitions that card to REJECTED instead.

Each pass writes one immutable `adoption_events` row and its `adoption_event_cards` rows in the same SQLite transaction as every card promotion. The event snapshots score, pass state, manifest hash, and backlog revision from `run.json`; each card row retains its `change_boundary_hash` and `application_id`. Adoption checks the explicitly reviewed versions, Owner, application IDs and boundary hashes in the same transaction; a mismatch rejects the entire selection. `all`, an omitted RUN, and `latest` are not accepted. `runs/outcomes.tsv` is an atomically regenerated projection, not a source of truth. The legacy adoption columns for RUN comparison remain readable for historical events; new passes do not obtain comparison data from the RUN manifest.

## Evidence and snapshots

APPLIED snapshots retain schema version 3 and now include `application_id` per card. New benchmark captures require non-empty application IDs. Historical snapshots without the field remain readable by Evidence and as comparison inputs, but cannot authorize adoption (including FORCE). They are never rewritten or backfilled.

The CLI assigns `application_id` as `<card ID>@<card version at entry>` on every transition into APPLIED, including reapplication with an unchanged Change boundary. Metadata/Owner/History updates do not change it. Existing databases migrate automatically; existing APPLIED cards receive `legacy:<ID>@<version>` for future captures, without claiming agreement with old RUNs. The ID identifies a recorded application, not a code hash: worker must still verify and record the actual deployed snapshot and must move work out of APPLIED before changing it.

| Hash | Input | Effect of a change |
| --- | --- | --- |
| `change_boundary_hash` | Normalized `Change boundary` | Invalidates comparison/adoption against the old snapshot and makes any historical assessment bound to the old boundary stale |
| `decision_hash` | `Hypothesis`, `Evaluation` | Review warning only; does not invalidate comparison/adoption or require a quantified gain |

All other fields, including workflow metadata, relations, observations, results, and History, are excluded. Line endings, trailing spaces, and equivalent JSON formatting are normalized. These hashes detect stale declarations; they do not prove deployed-code agreement or semantic equivalence of differently worded declarations.

To inspect evidence, the CLI selects the newest finalized RUN whose APPLIED snapshot actually contains the card. Use `--run` to select one explicitly. Endpoint, TSV, and profile comparisons prefer the card's `Compare Run`, set with `--compare-run` on create/update. With multiple explicit RUNs, the latest specified RUN is used. If unset, evidence selects the latest earlier finalized RUN with identical roles (including entry and additional roles). An invalid explicit selection does not fall back. This selects evidence for mechanism review, not a verified causal baseline; check source, load windows, workload, and concurrent changes before attributing improvement. Adoption records the selected evidence RUN and snapshot; use the card's Compare RUN or the analysis comparison views for descriptive comparisons.

```shell
task backlog -- evidence B-001
task backlog -- evidence --run 20260904-120000 B-001
task backlog -- evidence --format json --run 20260904-120000 B-001,B-002
```

`Evaluation` can contain a version 1 JSON extraction contract or free text. The evidence command extracts the specified endpoint/profile/TSV references for JSON; free text is displayed as checks without automatic metric extraction. Neither form automatically decides adoption. See [Evaluation reference](../../.agents/skills/_shared/evaluation.md) for supported fields, lookup limits, and how to retain decision criteria. Define available references when preparing READY cards; existing free-text cards do not require bulk conversion.

## Storage and validation

Every CLI mutation increments `backlog_revision` and dumps the database. Use the [writer protocol](backlog-workflow.md#writer-protocol); do not edit SQLite or its SQL dump manually.

When the CLI runs in the repository worktree, a mutation also commits only `tools/backlog/backlog.sql` with the message `chore(backlog): update backlog ledger`. Unrelated staged changes are preserved. If the dump is already staged, the database and dump are still updated but the automatic commit stops with an error so the staged choice can be committed manually.

```shell
task backlog -- validate
```

Validation checks SQLite integrity, IDs and versions, card states, READY contracts, dependencies, Objective hierarchy, ACTIVE Target scope and goals, primary links, Target relations, and historical assessment bindings.

For existing cards, migrate the old section with `task backlog -- migrate-evaluation B-001 --expect-card-version N --actor human:name --reason "Rename and scope evaluation" < evaluation.txt`. The input is the revised Evaluation text. This one-time operation preserves status, Owner, section order, and the previous text in History, including for terminal cards. Normal writes accept only `Evaluation`.

## Model migration

Target migration preserves existing A-IDs, Evidence, History, adoption decisions, and legacy relation/assessment records. Terminal historical Interventions can carry an explicit legacy-link exception; this remains an audit accommodation and is not a discovery or duplicate-check input. Current creation and READY permit unlinked Interventions while preserving the body and dependency gates. Objective organization and common validity conditions are documented in [Objective organization](objective-organization.md) and [Common validity conditions](common-validity.md).

A fresh template ledger contains no Objectives, Targets, Interventions, or adoption history. Establish Objectives from the current contest’s official rules and Evidence using `isucon-objective`. Opening the ledger does not seed contest-specific hypotheses.

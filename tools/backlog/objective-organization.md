# Objective / Target organization

The valid final score is the implicit top-level purpose. Objectives describe evidence-backed contribution hypotheses; Targets describe a concrete subject, evaluation axis, present condition, and improvement goal; Interventions describe changes evaluated and adopted together.

A fresh template starts with no Objectives, Targets, Interventions, or historical results. Use `isucon-objective` with the current contest’s official materials and available Evidence to establish Objectives. Do not copy another contest’s scoring mechanisms, awards, card IDs, or judgments into the new ledger. Shared validity requirements belong in [common-validity.md](common-validity.md).

## Existing ledger migration

When importing an existing ledger, the CLI migrates legacy Constraints to Targets while preserving IDs, versions, Evidence, historical judgments, and the original resolution as the goal. `INVALIDATED` becomes `RETIRED`; legacy tables and assessments remain available for migration/audit inspection. Existing links become `IMPROVES`, with their old roles retained separately. Review active Targets’ axes, evaluation conditions, and Objective links against current Evidence. Terminal Interventions (`VALIDATED`/`REJECTED`) are not inputs to current discovery or duplicate checks. Migration itself does not prove goal completion.

Record contest-specific reorganization and its evidence in that contest’s Backlog History and RUN artifacts. The template carries no such migration record. New Objectives are not automatically seeded, and opening an existing ledger does not overwrite its Objectives.

Target links on Interventions are optional. Without them, record the improvement goal, score-contribution hypothesis, Evidence, and evaluation conditions on the card itself. `isucon-analyze` still starts from an existing ACTIVE Target; `isucon-rethink` and material-based exploration may propose unlinked Interventions within their scopes.

See [workflow](backlog-workflow.md) for authority and state transitions, [CLI](README.md) for operations, and [skills](../../.agents/skills/README.md) for responsibilities.

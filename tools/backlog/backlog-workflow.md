# Objective / Target / Intervention workflow

## Authority

This is the source of truth for Backlog rules. Read only the sections required by the active skill. CLI usage and storage are in [README.md](README.md); evidence selection is in [Evidence](../../.agents/skills/_shared/evidence.md). Skill-specific limits stay in the responsible skill.

Skill ownership: `isucon-objective` manages Objectives; `isucon-target` discovers and manages Targets and their Objective links. `isucon-analyze` discovers Interventions for existing ACTIVE Targets. `isucon-rethink` explores system-wide structural alternatives using Objectives and Evidence without requiring Targets, and directly creates INVESTIGATE Interventions. `isucon-agent` investigates and proposes interactively with the user, creating INVESTIGATE Interventions only for candidates the user has agreed to; it applies no exhaustive-coverage completion gate. When the user explicitly directs it to implement or deploy a scoped change, `isucon-agent` may perform that implementation and deployment while preserving the ownership and lifecycle rules below. Discovery and investigation skills may link their Interventions to existing Targets, but do not create or revise Objectives or Targets. They report missing or questionable targets with Evidence to the responsible skill. Worker normally implements and deploys changes, including corrections and removal requested by verifier. `isucon-verifier` reviews the specified benchmark RUN, decides adoption or correction/rejection handoff, and closes a rejection-cleanup application as REJECTED after reviewing its post-cleanup RUN. Before handing back work, verifier reuses the original investigation and investigates only the correction delta within the same Change boundary to establish a concrete, justified implementation approach. Both report relevant Target evidence to `isucon-target`; neither transitions Targets.

Exception: `isucon-analyze` may append attainment observations to an existing Target's Evidence, preserving its prior contents and recording the facts, snapshot, and uncertainty. Target definitions, status, and Objective links remain unchanged; `isucon-target` owns the attainment decision.

Use `task backlog -- ...` and the Writer protocol; never edit SQLite or its SQL dump manually. Official behavior and validity come from `docs/official/`.

Contest-specific change authorization follows [AGENTS.md](../../AGENTS.md#isuconでの変更方針): destructive changes needed for the requested improvement are permitted, including recreating contest databases and removing obsolete code or configuration. Preserve explicit retention requirements, official correctness and persistence, and other ongoing work and Evidence.

Implementation work does not require a preplanned reversal procedure. Do not retain obsolete implementations or add backups, switching flags, or reverse migrations solely to undo a future change. Use available code and change history when a correction is needed, and remove obsolete code and files. This policy also applies to older card bodies and archived analysis records; their reversal plans are not current implementation requirements. Transaction aborts, correctness-required fallback paths, and recovery from a failed file upload retain their runtime semantics.

## Objective

The implicit overarching purpose is maximizing the valid final contest score. An Objective defines a contribution path to that score: which scoring opportunities it enables or advances, or which score losses it prevents. Derive these paths from official scoring semantics and current Evidence. Generic latency reduction and resource efficiency are means shared by multiple paths, not Objective categories on their own. Increased work or resource consumption is acceptable when the net effect is expected to improve the valid final score. Do not prescribe concrete example Objectives or a fixed catalog of paths that anchors discovery. Neither an immediate score gain nor a current bottleneck is required.

Separate Objectives when their contribution hypotheses can be supported or challenged independently; do not split mechanically by API or scenario, or merge merely because paths share a resource or optimization. Reassess existing titles, definitions, and boundaries from the observed paths rather than only appending Evidence to broad categories. A title must identify the contribution path and intended direction. When narrowing an Objective, account for the excluded paths through other Objectives or explicit unresolved or deferred findings. Historical organization records describe past decisions, not a required current taxonomy.

Record the intended direction, the assumed causal path to score, an evaluable metric/predicate, verification, and official sources. Use the existing metric/predicate and verification fields to state the hypothesis, its premises, and what would support or challenge it; a separate schema field is not required. Faster APIs or lower resource use alone do not prove a score improvement. Distinguish observed local effects from the score hypothesis, including adverse tradeoffs and workload/comparison limitations.

Objective describes why a family of improvements may help. Target specifies what to improve in this effort, its axis, baseline, and goal. Keep the causal path to score explicit rather than treating a local metric as the final purpose. Do not create grouping destinations merely to accommodate existing implementations.

```text
status: ACTIVE | RETIRED
mode: SATISFY | MAXIMIZE | MINIMIZE
priority: P0 | P1 | P2
```

Inspect ACTIVE Objectives before discovery. Do not create an Objective merely to accommodate existing Interventions. The implicit purpose is not an Objective card. An explicitly requested separate award criterion must be clearly distinguished from main-score outcomes. See [Objective organization](objective-organization.md) for template initialization and legacy migration guidance.

`priority` ranks the expected value of pursuing an Objective's score-contribution path next; it is not a lifecycle state, a Target priority, or a guarantee of score impact. Existing ledgers may contain an empty priority until `isucon-objective` reassesses that Objective. New Objectives and Objective reassessments should set an explicit priority and record the judgment in History.

## Common validity conditions

[Common validity conditions](common-validity.md) define the shared requirements. Official correctness, authentication/authorization, initialization, restart persistence and reproducibility requirements apply to every Intervention, regardless of links. Read the relevant `docs/official/` sources, identify applicable conditions in Evaluation, and verify them in the implementation/adoption workflow. They cannot be traded for score. Former validity Objectives remain historical records rather than active grouping destinations; retiring them does not weaken these conditions.

## Target

Targets express What: an observed outcome for a concrete object under stated conditions, such as latency for a particular API or CPU demand for a shared operation. Specificity does not make a goal How. Eliminating database round trips, copies, or repeated calculations selects a means and belongs in Intervention hypotheses, not Target goals. Select Targets from measurements and Objective contribution; isucon-target uses saved measurements and responsible-agent History rather than inspecting application code, configuration, or live hosts. A different implementation achieving the same outcome must be eligible for attainment. Mechanism changes alone do not demonstrate attainment.

A Target is the object, scope, axis, and goal of one improvement effort. Its axis and direction of change follow the linked Objective and Evidence, not a default preference for less work. Increasing work can itself be a Target when the expected net contribution to score supports it. Define the desired outcome for the object without selecting an implementation mechanism; mechanism selection belongs to Intervention exploration. Do not use concrete example Targets to constrain discovery. A current bottleneck is not required. A Target requires:

1. one or more ACTIVE Objective links with a causal rationale and exactly one primary Objective;
2. concrete scope and evaluation axis;
3. baseline Evidence and snapshot/premise (units and denominators for numbers);
4. a goal for this effort, numeric or otherwise objectively decidable;
5. evaluation conditions and a stable fingerprint.

Do not require special measurements or restart tests to resolve a Target. Common validity requirements remain part of implementation and adoption; known correctness failures must not be ignored. Missing special tests or individual score attribution alone must not prevent resolution when the Target's goal is met. Preserve previous goals and evaluation conditions in History when revising them.

Ground the goal in baseline Evidence, an official requirement, or an explicit user requirement. Goals may require an increase, decrease, or another objectively decidable change; select the direction from the score hypothesis. A local structural change alone is not a sufficient reason to define a Target around a proposed mechanism. Do not invent a numeric threshold merely to populate a field. If no defensible goal can be stated, retain the finding as unresolved rather than creating a placeholder Target. Unknown effect size does not prevent creating a Target. Separate confirmed mechanisms from estimated impact and future workload conditions. Reuse an existing Target when scope, premise, and goal match; do not create one per implementation option.

```text
status: ACTIVE | RESOLVED | RETIRED | MERGED
```

RESOLVED means Evidence demonstrates this effort's goal was attained, not that no improvement remains. Record the comparison conditions and result; Intervention adoption never resolves a Target automatically. Goal changes preserve the previous goal and reason in append-only History. Lack of candidates or reduced priority is not attainment; an applicable unmet goal remains ACTIVE. RETIRED means the goal no longer applies; MERGED points to the surviving duplicate. Terminal Targets do not reopen. Recurrence creates a new Target with a versioned fingerprint, a previous-Target link, and the changed premise or new requirement. Preserve historical decisions and evidence; never invent retrospective goals to claim past success.

## Intervention

An Intervention records one proposed implementation, application, and adoption boundary. Its B-xxx/B-xxxx ID (at least three digits) is its identity; there is no separate fingerprint. The boundary may be broad and may span multiple components; it is not a requirement that the proposal be small or independently deployable. New cards start in INVESTIGATE, where Change boundary may be omitted or provisional. Refine it before READY so the implementation, application, and adoption scope is concrete. Detect duplicates among open Interventions by target, mechanism, and the available Change boundary; an omitted or provisional boundary is not by itself a reason to reject a concrete hypothesis.

Apply the improvement principles in [AGENTS.md](../../AGENTS.md#目的と改善判断の原則): compare candidates against the current implementation, including remaining work and added costs. The current implementation and saved RUN Evidence are the baseline; do not use prior adoption or terminal-card decisions as a reason to stop looking for further improvement. Similar targets or mechanisms alone do not establish duplication. First identify the concrete incremental change; then decide whether it belongs in an existing open Intervention or constitutes a separately evaluable proposal. Only open Interventions (`INVESTIGATE`, `READY`, `DOING`, `VERIFY`, `APPLIED`, or `BLOCKED`) participate in duplicate checks and reuse of prior backlog investigation. `VALIDATED` and `REJECTED` are terminal records for lifecycle/audit purposes, not discovery or duplicate-check inputs. Preserve terminal records and ownership rules; report inseparable amendments when the current skill cannot edit the owning card. A duplicate decision disposes of that exact proposal, not the search for further improvement in the requested scope.

Split an existing open proposal only when its parts retain meaningful behavior and can be independently implemented and evaluated for adoption, not by file/service count, stages, or team size. This split rule does not require a new INVESTIGATE hypothesis to be small, independently deployable, or fully separable at creation.

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

### Ownership and handoff

A card has one writer at a time. Use a unique session identity as Owner, not a shared skill name. Claim an unowned card with both an empty-Owner precondition and its latest version; see [CLI examples](README.md#ownership-and-handoff). Check current status and scope before claiming. Owner changes, state transitions, and their reason must be atomic; on conflict reread and reassess rather than stealing another Owner. An older worker that still owns APPLIED must explicitly release it before verifier claims it.

- Worker normally owns READY implementation and its DOING/VERIFY work. An explicitly user-directed `isucon-agent` implementation is an exception for the requested scope; it must not take an existing Owner or silently change card lifecycle state. After successful deployment and production checks, the normal worker path transitions to APPLIED and releases Owner atomically. Each entry into APPLIED generates a new `application_id`; record it with the deployed snapshot.
- Verifier claims only eligible APPLIED cards from the specified RUN snapshot. Pin the RUN, application ID, Change boundary, Evaluation, and Compare Run. Adoption uses the reviewed Owner and versions. Verifier never edits code, deploys, or takes READY work.
- Before limited correction or rejection cleanup, verifier investigates the affected delta to the same depth as investigation: connect observations to code and conditions, compare relevant remedies and their runtime costs, select a feasible approach, and check its correctness premises. Reuse unaffected prior investigation on that open card. Record the concrete changes, selection evidence, retained behavior, implementation checks, post-application evaluation, and remaining uncertainty in the card History before releasing ownership. Preserve the reviewed application declarations; worker updates the body for the correction. Measured effect size and complete causal proof are not prerequisites, but an unexplored list of problem areas is not a completed handoff.
- For limited correction or rejection cleanup, verifier transitions APPLIED to unowned DOING atomically with the decision, RUN/application ID, attribution, investigated correction approach (or a reference to its prior History entry), changes to retain, and completion criteria recorded in History. History is the handoff record; no separate handoff field is required. Verifier stops writing that card after the transition.
- Worker claims unowned DOING before new READY. Limited correction returns to APPLIED with a new application ID after deployment/checks. Rejection cleanup also returns to APPLIED with a new application ID after the required correction/removal and checks/deployment are complete; this cleanup application waits for the next user benchmark, and verifier closes it as REJECTED after reviewing that RUN. A stopped worker leaves owned unfinished work for explicit resumption or transfer; it is not automatically reclaimed.

The CLI preserves dependency gates. If a handoff would invalidate a dependent card, inspect the affected dependency and coordinate its owner's state correction first; do not weaken or remove dependencies just to force a handoff. Report an unresolved dependency/ownership conflict as uncompleted handoff, not a completed rejection.

Verifier may inspect saved RUN artifacts alongside worker implementation, but must not treat live SSH state as historical RUN evidence. Before deployment, confirm the worktree, Taskfile, target hosts and active RUN; no deploy/restart/reset may overlap an active measurement. Retain AGENTS.md's checks protecting measurements across environments.

APPLIED has no count limit. Unreviewed applications and corrected cards awaiting a manual benchmark do not stop acquisition or deployment. Batch boundaries come from dependency/application coherence, available work, urgent correction, execution constraints, or explicit user scope/stop instructions.

### READY gate

This gate applies when an Intervention enters READY and to every later state that depends on the READY contract. It does not apply to creation in INVESTIGATE. At creation, a Change boundary may be empty or provisional; the creating skill should record the known scope and unresolved boundary questions when available. READY requires the final scope and the three sections below to be non-empty.

The card body must make three decisions clear without a separate generic contract schema:

1. `Hypothesis` — concrete improvement scope, intended outcome, and causal path to the valid final score; when linked, explain contribution to the Target and its Objective
2. `Change boundary` — implementation and application scope
3. `Evaluation` — post-application observations from saved measurement results or read-only SSH, and adoption/correction/rejection criteria

Before entering READY, confirm that the title summarizes the card's final Intervention, meaning the change to pass to implementation, application, and evaluation.

Evaluation is planned during INVESTIGATE and used after application, once measurement results are available. It is the adoption decision contract: include only conditions whose result can change adoption, limited correction, or rejection, and connect each condition to the relevant Change boundary, Evidence, comparison conditions, and decision. Do not use it as an exhaustive investigation, diagnostic, implementation-test, or deploy checklist; record those observations in `Unknowns`, `Result`, `History`, or saved Evidence. Official validity conditions applicable to the boundary remain required. There is no fixed count or size limit; a broad boundary may require a broad Evaluation, but every condition must have a stated decision effect. Test execution, deploy, restart, and benchmark execution belong to the implementation/measurement workflow, not this section. `VERIFY` remains the pre-deploy lifecycle state.

For artifact selectors, machine-readable references, and conditions requiring human review, see [Evaluation reference](../../.agents/skills/_shared/evaluation.md). The extraction format does not replace the criteria above or the adoption requirements below; existing free-text evaluations remain supported.

New cards always start in INVESTIGATE. Target links are optional at creation, READY admission, and subsequent stages. A Change boundary may be missing or empty while a card is in INVESTIGATE; the creating skill should record Hypothesis and Evaluation content when it is available. All three READY sections must be populated before READY. When linked, READY admission requires a primary ACTIVE Target connected to a primary ACTIVE Objective; explain the fit of every link. Satisfy BLOCKING dependencies before READY; ORDERING dependencies need not be complete. `isucon-investigate` is the only skill that creates READY and confirms the three sections in one `resolve` operation.

The skill judges these decisions using standard Evidence or correctness checks; the CLI validates structure only (see [CLI checks](README.md#cli-checks)). Unknown effect size, absence of resource saturation, or no direct measured gain does not prevent READY when causal direction and evaluation are explainable. This includes non-bottleneck improvements, selection, loss recovery, spam control, and experiments. “Try it and inspect score” is insufficient.

### BLOCKED

BLOCKED requires a concrete external fact, permission, environment state, or artifact unavailable from repository Evidence. In the body or History, record the question, inspected sources, facts, minimum missing information, acquisition route, evidence baseline, resume trigger, and resume state. No dedicated JSON contract is needed.

Low confidence, incomplete investigation, unknown effect size, and desire for measurement are not BLOCKED.

### Adoption

Use [the explicit RUN/Owner/version adoption command](README.md#adoption-records) to promote only ordinary APPLIED applications from the target RUN's before-bench snapshot to VALIDATED. An APPLIED application explicitly recorded in History as a `rejection-cleanup application` is excluded: it must not be adopted, and verifier closes it as REJECTED after reviewing the post-cleanup RUN. APPLIED after a limited correction remains an ordinary application and is eligible for VALIDATED when the adoption conditions are met. Require:

- finalized RUN, `passed=true`, known score;
- usable snapshot, matching non-empty application ID, and unchanged Change boundary declaration;
- the reviewing Owner and explicitly supplied current card versions still match inside the adoption transaction;

`task pass FORCE=true` is an explicit, reasoned exception recorded in History. It bypasses only pass and known-score checks, never finalization, snapshot integrity, application/Change boundary matching, or Owner/version checks, and it cannot adopt a `rejection-cleanup application`.

Adoption events are authoritative for adoption; `run.json` for the RUN. Persistence and derived outputs: [Adoption records](README.md#adoption-records).

### Rejection during investigation

REJECTED requires Evidence of an already resolved proposal, official-spec violation, unexplainable causal direction, duplicate open Intervention, or technical refutation. Effort, size, or a single RUN's score alone are insufficient.

An already resolved proposal means the exact proposed change is already satisfied in the current implementation. It does not include further improvements merely because they concern a previously optimized area. Record the current-versus-proposed comparison supporting this decision.

### Rejection after application

Before investing in another correction after an attributed regression, reassess whether the Intervention can yield a net contribution to the valid final score over its pre-intervention state. The case for continuation must be supported by Evidence: reducible work and waiting, remaining and newly added runtime costs, and their effects on scoring opportunities. Exact score forecasts, measured net gains and complete causal proof are not required. A repairable operation, partial recovery from the preceding RUN, or absence of a complete refutation is not sufficient. After examining the available relevant Evidence, default to rejection if a credible net-benefit case cannot be supported. Distinguish insufficient grounds for further investment from proof that improvement is impossible; a single score fluctuation or incomparable RUN alone is not grounds for rejection.

Temporary regression may be accepted as a continuation decision when concrete follow-up open Interventions make the overall improvement credible. Record their card IDs, why the current change is necessary, the mechanism and feasibility Evidence, combined costs and expected scoring benefit, and evaluation conditions for the whole effort. Explain what happens to the current change if the follow-ups fail or are abandoned. Uninvestigated future ideas or generic infrastructure value do not justify continuation. Official validity is never relaxed. This does not authorize verifier to discover or create follow-up cards, change other owners' work, merge independent adoption boundaries, or treat expected future gains as observed gains. Adoption still requires the current card's Evaluation and the Adoption conditions; a continuation decision alone does not establish VALIDATED.

On each subsequent review, assess the previously recorded continuation conditions against the results. Retain the pre-intervention baseline for the overall decision and distinguish it from the preceding revision used to assess a correction. Record differences in roles, source, load and concurrent changes; do not replace the selected baseline or infer causality from incompatible comparisons. Record what next results warrant adoption, further investment or rejection. Do not move the baseline or replace unmet conditions with another repair idea merely because partial recovery occurred. Revise conditions only when new Evidence changes the overall net-benefit case, explaining why. There is no fixed retry count; each additional investment needs this justification.

Only after supporting continuation should verifier concretize a limited correction within the same Change boundary. Otherwise, investigate the necessary rejection cleanup. For rejection, record the RUN, application ID, attribution, net-benefit assessment and cleanup conditions while handing APPLIED back to unowned DOING. Worker performs the required correction/removal through normal deployment and correctness checks, then returns the card to APPLIED with a fresh application ID and records that this is a rejection-cleanup application awaiting a user benchmark. Verifier reviews that post-cleanup RUN, does not adopt the cleanup application, and transitions APPLIED to REJECTED when the cleanup conditions are satisfied. If no code or deployment change is needed, worker records why and the supporting checks before returning the card to APPLIED for the same verifier review. Failed or unfinished cleanup remains DOING/VERIFY (or a justified BLOCKED), never prematurely REJECTED. Rejection does not require restoring a previous implementation.

Correctness failure, official-spec violation, operational failure, or causal refutation can justify rejection; mechanism degradation and correctness violation need not both exist. A single score fluctuation alone never justifies rejection.

## Relations

Use Objective ← Target ← Intervention when Targets apply. Target-to-Objective links are required; Intervention-to-Target links are optional throughout the lifecycle. Links may be multiple, and each non-empty set has exactly one primary destination. Record the causal rationale for each link. Use a fitting existing Target when available; do not create a placeholder Target or remove a meaningful link just to bypass review. Without a Target, record the improvement scope, intended outcome, score contribution hypothesis, and available baseline Evidence in the Intervention itself. At INVESTIGATE creation, details may be provisional, and Change boundary may be omitted or empty; explain the observed mechanism, why the proposed change is likely to improve it, and the supporting Evidence. Consider major added costs and premises; distinguish facts, inference, and open questions. Known effect size, a finalized Change boundary, and objectively decidable adoption criteria are not creation prerequisites; establish the concrete scope and adoption criteria before READY. Missing Target links alone do not prevent creation or READY. Historical terminal records are preserved without fabricating links, but they are not discovery or duplicate-check inputs.

Target-driven discovery such as `isucon-analyze` continues to investigate existing ACTIVE Targets and links its candidates to those Targets. `isucon-rethink` does not use Target discovery or matching as a prerequisite and creates unlinked proposals; investigation may link fitting Targets later. Source-driven discovery such as `isucon-use-solution` and `isucon-special-sauce` may create unlinked candidates when no existing Target fits.

An Intervention targets improvement; it does not promise to resolve the entire Target. The `IMPROVES` relation replaces `RESOLVES` / `MITIGATES` and mandatory residual assessments as the common improvement relation. Target goal attainment is evaluated separately. Direct Intervention-to-Objective links are unnecessary; trace contribution through Targets. Link or status changes never automatically change related card states.

Dependencies are explicit `ORDERING` or `BLOCKING` relations and always specify `required-status`. Change-boundary overlap and short-lived implementation order are not persisted dependencies.

## Evidence policy

RUNs, measurements, logs, profiles, code, configuration, and official sources are Evidence, not card kinds. Use existing Evidence; record gaps in Target or Intervention History, not measurement-only cards. Do not require extra measurement when READY conditions are already explainable. New or changed standard instrumentation requires a separately authorized repository task.

Selection and comparison: [Evidence](../../.agents/skills/_shared/evidence.md). CLI behavior and snapshot hashes: [README.md](README.md#evidence-and-snapshots).

## Priority

`isucon-objective` assigns or revises a justified P0, P1, or P2 for every new or reassessed Objective, preserving an explicit user priority and briefly recording the reason in History. Compare contribution paths using expected valid-score opportunity or loss avoided, Evidence, uncertainty, added runtime costs, and the conditions under which the ranking would change. A high Objective priority does not automatically make every linked Target or Intervention high priority; each downstream effort is ranked independently. Objective priority guides review order and does not permit skipping the full ACTIVE Objective or Target scope.

`isucon-rethink` assigns a justified P0, P1, or P2 when creating or updating its eligible INVESTIGATE proposals, using the criteria below and briefly recording its reasoning. Structural scope or discovery origin alone does not imply P0. This assessment does not bypass READY gates or ownership and work-in-progress rules; investigation still assesses Priority against the final proposal. Structural proposals need Evidence supporting the expected improvement direction, but not measured gains or a finalized design; neither proposal count nor large-change count is a goal.

For `isucon-investigate`, setting Priority is mandatory whenever moving an Intervention to READY. Use the final card content, including after splitting, merging, or renaming, and pass `--priority` in the same `resolve --status READY` operation. This is a skill requirement, not a CLI gate.

Investigation also supplies `--implementation-estimate-minutes` in that READY resolution, based on the final Change boundary and the normal work path of `isucon-worker`. The value is one rough integer for implementation, local checks and build, deployment, and worker-side remote checks; do not add ranges, uncertainty allowances, or conservative human-equivalent padding. Revise the estimate when the boundary changes. See [Implementation time estimates](README.md#implementation-time-estimates) for scope, units, and unset semantics. Estimates inform planning without replacing the ranking criteria below.

Target Priority may be set or revised by `isucon-target`; Intervention Priority may be set or revised by `isucon-analyze`, `isucon-rethink`, `isucon-agent`, and `isucon-investigate` within their existing editing scope. Assign it during ordinary creation or updates, including when an existing value is blank. Use P0 (high), P1 (normal), or P2 (low) as a rough guide; when unsure, use P1. A quick subjective judgment is sufficient: do not spend time on additional research, quantitative scoring, or exhaustive comparisons solely to assign Priority. Preserve explicit user priorities. Priority does not replace Evidence or lifecycle gates.

1. existing DOING, APPLIED awaiting verification, attributable defects requiring correction;
2. violations of common validity conditions;
3. explicit user priority;
4. expected contribution to score from the Intervention hypothesis, through ACTIVE Targets and their Objectives when linked, considering Evidence, added runtime costs, uncertainty, and unmet dependencies.

Resource bottlenecks have no automatic precedence over scenario latency, loss reduction, or other improvements. Target goal attainment and expected score contribution guide ranking; candidate count is not a goal. Use the shared [Evidence comparison criteria](../../.agents/skills/_shared/evidence.md#改善方向と次の投資判断) for both local and structural proposals, including the distinction between runtime costs and development effort. Ease or size must not narrow exploration before the structure and improvement mechanism are considered. Adoption, Target attainment, Objective support, and priority for further investment are separate decisions. Reuse existing Evidence and rough mechanism-based estimates; these criteria add no measurement gate or fixed improvement threshold.

Owner, dependency, conflicts with existing uncommitted changes, and deployment snapshot coherence take precedence over candidate ranking. Within a class, use Priority and the completed snapshot.

### Work selection

Worker uses all READY cards, including newly READY cards, unless the user explicitly limits the scope. Use Priority and Evidence to choose implementation order, not to exclude cards by selecting a narrower scope. Recheck the latest list after each implementation review, after each deployment batch, and immediately before finishing. Prioritize owned unfinished work and claimable unowned DOING returned by verifier before new READY. Continue with eligible cards regardless of APPLIED count; preserve ownership, dependencies, conflicts, and user stop instructions.

Verifier reviews only the specified RUN snapshot and ends after adoption decisions, rejection-cleanup closures, or correction/rejection handoffs for eligible cards. It does not acquire newly APPLIED cards outside that snapshot, implement READY, or automatically invoke worker. Report ownership/application mismatches and concrete external blockers separately from completed decisions.

Investigate uses all INVESTIGATE cards, including newly created cards, unless the user explicitly limits the scope. Use Priority and Evidence to choose investigation order and questions, not to exclude cards by selecting a narrower scope. Recheck the latest list at the start, after each card, and immediately before finishing; continue while eligible cards remain. Preserve ownership, dependencies, conflicts, and the rule against reclaiming management referrals without changed premises. Resolve each decided card promptly without waiting for unrelated investigations.

Analyze uses all ACTIVE Targets unless the user explicitly limits the scope. Use Priority, Evidence, and recommendations from earlier stages to choose investigation order and questions within each Target, not to exclude Targets from detailed exploration. Before finishing, reread the ACTIVE Target list and confirm detailed findings or justified reuse of prior detailed investigation attached to the ACTIVE Target or an open Intervention for every in-scope Target, including new arrivals. Overview-only deferral is insufficient. Reuse must identify prior Evidence and questions, assess current differences and reconsideration conditions, and address unresolved questions that can be advanced with permitted resources. Ownership restricts card mutations, not read-only exploration of the Target.

The scope-selection rules below apply to other discovery skills, not analyze, investigate, or worker. Recommendations passed to analyze guide its order and questions; only an explicit user restriction narrows its Target scope.

Separate an overview of the requested domain from detailed exploration and new work. The parent agent coordinating the request selects the current scope from supported score-contribution hypotheses and the latest reassessment. Pass that scope, its rationale, relevant IDs and snapshot to the responsible skills. For a standalone invocation without a supplied selection, that skill's parent makes the selection from available Evidence within its own authority; missing selection is not a reason to wait or invoke another skill.

State which hypotheses or questions receive detailed work now, why prior results support continuing or changing focus, and what is deferred with a reason and reconsideration condition. Use relevant ACTIVE Target/open Intervention History and RUN artifacts; no new entity, schema, exhaustive comparison table, or fixed candidate count is required. ACTIVE or READY status alone does not select an item for this invocation. Deferral does not resolve a Target, reject an Intervention, or change ownership. Preserve explicit user scope, including requests to process every specified item, and prioritize required corrections and completion of work already undertaken.

Discovery skills reassess the selection when new Evidence changes its premise; do not expand it merely because more cards exist or APPLIED capacity is available. Conversely, a selected scope is not permanently closed to a newly supported contribution. Selection must not introduce per-card benchmarks, fixed small batches, or additional measurement prerequisites. Skills retain their existing authority, and selecting work does not authorize automatic invocation of another skill.

## Writer protocol

- Read current entity version immediately before writing.
- Include `--actor` and `--reason` on every mutation.
- Use `--expect-card-version`, `--expect-target-version`, or `--expect-objective-version`. Ownership claims/releases also use `--expect-owner`; adoption supplies every reviewed version through `VERSIONS`.
- For primary-link changes, check both the destination and the owner of the shared link set: Objective + Target versions for `objective link --primary`, Target + Intervention versions for `target link --primary`. Relation mutations increment both versions.
- On conflict, reread and merge intentionally; do not overwrite.
- Keep History append-only.
- Finish with `task backlog -- validate`.

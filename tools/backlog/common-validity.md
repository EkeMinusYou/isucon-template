# Common validity conditions

These conditions apply to every Intervention. They are not optional Objective links and cannot be exchanged for a higher score.

The source of truth is the current contest’s rules, application manual, and API specification saved in [official materials](../../docs/official/). Check the relevant sections for each change; this document does not replace their requirements.

- Preserve API behavior, authentication/authorization, data consistency, and required read/write semantics.
- Preserve all initialization behavior, including database state and any dependent services required by the contest. Official initialization and consistency checks must pass; changing storage structures does not remove these obligations.
- Preserve startup and service configuration so the application operates after all servers restart. Apply the current contest’s restart and score-reproduction requirements; do not import a threshold from another contest.
- Preserve data written during a load run so it remains retrievable after restart, as required by official final verification. Process-local caches do not replace durable state.
- Preserve the official server environment and ability to perform final verification, including protected services, files, users, and access requirements.

During investigation, identify affected conditions in Evaluation and explain how the proposed boundary preserves them. During implementation and adoption, record available correctness and operational Evidence, distinguishing unverified restart/reproduction conditions from confirmed results. Do not claim that one successful RUN proves every condition.

Benchmark execution remains prohibited for agents. Request user execution when required; analysis and investigation do not deploy, restart, or execute benchmarks. Use the authorized implementation/measurement workflow for operational verification.

Handle a limited repair within the same Intervention and Change boundary, following the implementation and correction workflow. If Target links already exist, explain how restoring validity supports those Targets and their Objectives. Creating a Target or adding a Target link is not a prerequisite for repair; unlinked Interventions can be corrected through the same workflow. Do not create a catch-all score/validity Objective or imply that a score increase compensates for missing persistence.

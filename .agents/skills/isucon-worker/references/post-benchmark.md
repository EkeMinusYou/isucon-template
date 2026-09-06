# Post benchmark

1. 対象RUNの`run.json`がfinalizedで、対象カードがbefore-bench APPLIED snapshotに含まれることを確認し、pass/fail、score、artifact statusを読む。失敗RUNも確認対象とする。
2. failまたはcorrectness errorなら、採点改善より復旧を優先する。
3. カードのEvaluationと[EvidenceのRUN規則](../../_shared/evidence.md#runの扱い)に従い、保存済み計測結果と必要最小限の読み取り専用SSHで適用後の状態を確認し、採用・修正・不採用を判断する。比較不能でも対象RUN単独での問題確認・限定修正は進める。
4. Objectiveへの結果、guardrail、対象機構を分けて判定する。

問題がなく[workflowの採用条件](../../../../tools/backlog/backlog-workflow.md#adoption)を満たす場合は、`task pass`で対象APPLIEDをVALIDATEDにする。効果量などの結果が曖昧でも、公式仕様・明示された要求への適合と改善の方向が維持され、同じ採用条件を満たすなら、判断根拠と不確実性をHistoryへ残してVALIDATEDにする。例外採用も同節のFORCE条件に従う。

問題がある場合は、同じ変更境界で`APPLIED -> DOING`へ戻して限定修正し、検証後に`VERIFY`、正規deployとproduction状態確認後に`APPLIED`へ進める。修正後は次の手動ベンチで再判定し、それまではVALIDATEDにしない。

rollback・REJECTEDの判断は[Rejection after application](../../../../tools/backlog/backlog-workflow.md#rejection-after-application)、復旧操作は[workerの障害復旧](../SKILL.md#障害復旧)に従う。

Constraintの状態は[workflowのConstraint規則](../../../../tools/backlog/backlog-workflow.md#constraint)に従い、解消条件の現在値から確認する。

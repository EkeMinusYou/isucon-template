# Post benchmark

1. 対象RUNの`run.json`がfinalizedで、対象カードがbefore-bench APPLIED snapshotに含まれることを確認し、pass/fail、score、artifact statusを読む。失敗RUNも確認対象とする。
2. failまたはcorrectness errorなら、採点改善より復旧を優先する。
3. カードのVerificationと[EvidenceのRUN規則](../../_shared/evidence.md#runの扱い)に従って成果物を確認・比較する。比較不能でも対象RUN単独での問題確認・限定修正は進める。
4. Objectiveへの結果、guardrail、対象機構を分けて判定する。

問題がなく[workflowの採用条件](../../../../tools/backlog/backlog-workflow.md#validated-and-rejected)を満たす場合は、`task pass`で対象APPLIEDをVALIDATEDにする。例外採用も同節のFORCE条件に従う。

問題がある場合は、同じ変更境界で`APPLIED -> DOING`へ戻して限定修正し、検証後に`VERIFY`、正規deployとproduction状態確認後に`APPLIED`へ進める。修正後は次の手動ベンチで再判定し、それまではVALIDATEDにしない。

rollback・REJECTEDの判断はworkflowの採否規則に従い、復旧操作は[workerの障害復旧](../SKILL.md#障害復旧)に従う。結果が曖昧でも安全で方向が維持されるなら、比較不能理由をHistoryへ残して次の判断へ送る。

Constraintの状態は[workflowのConstraint規則](../../../../tools/backlog/backlog-workflow.md#constraint)に従い、解消条件の現在値から確認する。

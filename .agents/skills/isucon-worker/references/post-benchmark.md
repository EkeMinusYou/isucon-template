# Post benchmark

1. 対象RUNの`run.json`がfinalizedで、対象カードがbefore-bench APPLIED snapshotに含まれることを確認する。
2. failまたはcorrectness errorなら、採点改善より復旧を優先する。
3. 比較可能なRUNと、カードのVerificationに書かれた標準Evidenceを確認する。
4. Objectiveへの結果、guardrail、対象機構を分けて判定する。

採用は`task pass`で行う。単一RUNのscore変動だけで棄却しない。対象機構の悪化、correctness違反、他変更へ帰属しない原因が揃う場合は、rollbackしてEvidence付きでREJECTEDにできる。結果が曖昧でも安全で方向が維持されるなら、比較不能理由をHistoryへ残して次の判断へ送る。

ConstraintのstatusはInterventionの採否から自動推定しない。解消条件の現在値が成立した場合だけRESOLVEDへ進める。

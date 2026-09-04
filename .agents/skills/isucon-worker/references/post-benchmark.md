# Post benchmark

1. 対象RUNの`run.json`がfinalizedかつ`passed=true`、score既知で、対象カードがbefore-bench APPLIED snapshotに含まれることを確認する。
2. failまたはcorrectness errorなら、採点改善より復旧を優先する。
3. `run.json.comparison`で宣言されたcontrol RUNがある場合は`compatible`であることを確認し、そのRUNと、カードのVerificationに書かれた標準Evidenceを比較する。単なる直前RUNをcontrolにしない。
4. Objectiveへの結果、guardrail、対象機構を分けて判定する。

採用は`task pass`で行う。app journal、nginx error、kernel/OOMも確認し、単一RUNのscore変動だけで棄却しない。対象機構の悪化、correctness違反、他変更へ帰属しない原因が揃う場合は、rollbackしてEvidence付きでREJECTEDにできる。結果が曖昧でも安全で方向が維持されるなら、比較不能理由をHistoryへ残して次の判断へ送る。
通常の採用条件を意図的に上書きする場合だけ`task pass FORCE=true`を使う。forceでもfinalized RUN、利用可能なAPPLIED snapshot、カード定義一致は省略せず、強制採用であることをEvidenceに残す。

ConstraintのstatusはInterventionの採否から自動推定しない。解消条件の現在値が成立した場合だけRESOLVEDへ進める。

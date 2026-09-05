# Post benchmark

1. 対象RUNの`run.json`がfinalizedで、対象カードがbefore-bench APPLIED snapshotに含まれることを確認し、pass/fail、score、artifact statusを読む。失敗RUNも確認対象とする。
2. failまたはcorrectness errorなら、採点改善より復旧を優先する。
3. カードのVerificationに書かれた標準Evidenceを確認する。`run.json.comparison`で宣言されたcontrol RUNがあり、`compatible`ならそのRUNと比較する。比較不能でも問題の確認・限定修正は進め、単なる直前RUNをcontrolにしない。
4. Objectiveへの結果、guardrail、対象機構を分けて判定する。

app journal、nginx error、kernel/OOMも確認する。問題がなく採用条件を満たす場合は、`passed=true`、score既知、宣言されたcontrol RUNがある場合は`comparison.status=compatible`を確認し、`task pass`で対象APPLIEDをVALIDATEDにする。

問題がある場合は、同じ変更境界で`APPLIED -> DOING`へ戻して限定修正し、検証後に`VERIFY`、正規deployとproduction状態確認後に`APPLIED`へ進める。修正後は次の手動ベンチで再判定し、それまではVALIDATEDにしない。

単一RUNのscore変動だけで棄却しない。対象機構の悪化、correctness違反、他変更へ帰属しない原因が揃う場合は、rollbackしてEvidence付きでREJECTEDにできる。結果が曖昧でも安全で方向が維持されるなら、比較不能理由をHistoryへ残して次の判断へ送る。
通常の採用条件を意図的に上書きする場合だけ`task pass FORCE=true`を使う。forceでもfinalized RUN、利用可能なAPPLIED snapshot、カード定義一致は省略せず、強制採用であることをEvidenceに残す。

ConstraintのstatusはInterventionの採否から自動推定しない。解消条件の現在値が成立した場合だけRESOLVEDへ進める。

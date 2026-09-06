# Post benchmark

1. 対象RUNの`run.json`がfinalizedで、対象カードがbefore-bench APPLIED snapshotに含まれることを確認し、pass/fail、score、artifact statusを読む。失敗RUNも確認対象とする。
2. failまたはcorrectness errorなら、採点改善より復旧を優先する。
3. カードのEvaluationと[EvidenceのRUN規則](../../_shared/evidence.md#runの扱い)に従い、保存済み計測結果と必要最小限の読み取り専用SSHで適用後の状態を確認し、採用・修正・不採用を判断する。比較不能でも対象RUN単独での問題確認・限定修正は進める。
4. Objectiveへの結果、guardrail、対象機構を分けて判定する。

問題がなく[workflowの採用条件](../../../../tools/backlog/backlog-workflow.md#adoption)を満たす場合は、`task pass`で対象APPLIEDをVALIDATEDにする。効果量などの結果が曖昧でも、公式仕様・明示された要求への適合と改善の方向が維持され、同じ採用条件を満たすなら、判断根拠と不確実性をHistoryへ残してVALIDATEDにする。例外採用も同節のFORCE条件に従う。

問題がある場合は、同じ変更境界で`APPLIED -> DOING`へ戻して限定修正し、検証後に`VERIFY`、正規deployとproduction状態確認後に`APPLIED`へ進める。修正後は次の手動ベンチで再判定し、それまではVALIDATEDにしない。

rollback・REJECTEDの判断は[Rejection after application](../../../../tools/backlog/backlog-workflow.md#rejection-after-application)、復旧操作は[workerの障害復旧](../SKILL.md#障害復旧)に従う。

Constraintの状態は[workflowのConstraint規則](../../../../tools/backlog/backlog-workflow.md#constraint)に従い、解消条件の現在値から確認する。

## 比較RUNの選択

カードの`Compare Run`（`task backlog -- update <ID> --compare-run <RUN_ID>`で設定。通常のversion・actor・reason指定も必要）があれば優先する。空なら、対象RUNより前で`run.json`のrolesが同一の最新finalized RUNを使う。`task backlog -- evidence --run <対象RUN> <ID>`もこの規則で比較値を表示する。複数のCompare Runがあるカードは最新の指定RUNを使う。指定RUNが存在しない、対象RUN自身、または候補がない場合は比較なしとし、別RUNへ暗黙に切り替えない。

これはカードの機構指標を読む比較基準で、変更前RUNへの自動固定ではない。変更前を基準にし続ける必要がある場合は`Compare Run`を明示する。選択後は役割・source・計測窓・負荷条件・同時変更を確認し、比較不能な値を改善としない。採用判定とスコア差分に使うmanifestのcontrolは別に確認し、`task pass`の互換性条件をこの選択で迂回しない。

## 改善結果の表示

`task pass`の前に、各カードのHistoryへ、Change boundaryが減らした仕事、対象RUN・比較RUNの機構指標、correctness、帰属の限界を対応付けて記録する。複数カードを同時適用したRUNでは個別寄与と断定せず、合算効果または個別寄与は未分離と明記する。

ベンチ後の完了報告では、比較可能な成果物から改善を確認できたInterventionを次の形式で示す。カードごとに比較RUNが異なる場合は比較の組ごとに表を分ける。

```text
比較: <比較RUN> → <対象RUN>
| カード | 確認できた改善 |
| --- | --- |
| B-001 | <減らした仕事>。<機構指標>が <比較値> → <対象値>（<差分率または差分量>） |
```

- SQL、I/O、copy、lock、parse、往復など実際に減らした仕事と、単位・母数を揃えた比較値と対象値を書く。profile上のcallee消失など構造的な改善も対象範囲と根拠を示す。
- 正常性の列や未観測・横ばい・悪化した値は改善表に載せない。採否判断に必要な事実・不確実性はHistoryと表外の報告に残す。
- `VALIDATED`への遷移やスコア差だけをカード固有の改善証拠にしない。確認できた改善がなければ、表を作らず「比較RUNに対して確認できた改善はありません」と理由を報告する。改善表に載らないこと自体は採用を妨げない。

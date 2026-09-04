---
name: isucon-analyze
description: ISUCONの公式仕様、コード、設定、保存済みRUNを横断し、得点要素・penalty・validity・性能・ベンチ挙動・構成の観点からObjective、現在のConstraint、未被覆のIntervention候補を発見してINVESTIGATEへ引き渡す。READY化、実装、デプロイ、ベンチ実行には使わない。
---

# ISUCON analyze

依頼とEvidenceに必要な観点だけを選び、Objectiveに効く未被覆Interventionを発見する。性能ボトルネックだけに探索を限定しない。

## 最初に読む

- `AGENTS.md`
- `docs/reports/README.md`
- [Objective / Constraint / Intervention](../_shared/objective-constraint-intervention.md)
- [Evidence](../_shared/evidence.md)
- 対象に関係する`docs/official/`

次のreferenceは対象に応じて読む。

- リソース、待ち、処理量: [performance.md](references/performance.md)
- 得点要素、損失、成功、penalty、selection: [score-mechanics.md](references/score-mechanics.md)
- シナリオ進行や閉ループ: [benchmark-behavior.md](references/benchmark-behavior.md)
- 役割配置や移設: [topology.md](references/topology.md)
- `docs/special-sources/`や`docs/solutions/`: [known-solutions.md](references/known-solutions.md)

## 境界

- 保存済みEvidence、コード、設定、公式資料、必要最小限の読み取り専用SSHを使う。
- 実装、設定変更、deploy、service操作、bench、成果物の再集計は行わない。
- レポートとBacklog更新は親エージェントだけが行う。
- 情報不足や追加計測をカード化しない。標準Evidenceで方向を説明できない案はレポート上の未確定候補に留める。
- `VALIDATED`と`REJECTED`の過去判定を現在候補の自動棄却根拠にしない。現行コード・設定・premiseを再確認する。

## 手順

1. `task backlog -- objective list`、`constraint list --all`、open Interventionを読み、探索対象と未被覆範囲を決める。
2. ユーザー指定RUNを使う。指定がなければfinalizedな最新RUNを選び、`run.json`でsource・役割・artifact statusを確認する。
3. 公式採点仕様から、対象Objectiveに至る成功・失敗・得点・penalty・selection・時間の経路を必要な深さだけ分解する。
4. Evidenceから事実を抽出し、単位・母数・snapshotを保つ。コードと設定で発生機構と効果の方向を確認する。
5. 現在Objectiveを制約する事実が作成条件を満たす場合だけConstraintを作成・更新・終端する。候補がないことをConstraintの終端理由にしない。
6. 一体の採否・rollback境界ごとに未被覆Intervention候補を作る。ConstraintがなくてもObjectiveへ直接効く候補を含める。
7. 各候補をOwnerなし`INVESTIGATE`として起票し、Objectiveへlinkする。Constraintに正方向なら`RESOLVES`または`MITIGATES`を付ける。
8. 必要なレポートを`docs/reports/README.md`の命名規則に従って保存し、解析日時、事実、推論、
   未確定点、起票IDを分ける。

## 起票の最小内容

- Observation: Evidence、snapshot、単位・母数
- Hypothesis: Objectiveへ効く因果と期待する方向
- Change boundary: `pending`でもよいが、想定する採否境界を示す
- Verification: 既存の標準Evidenceで何を確認するか
- Fingerprint: target、mechanism、premiseを識別できる値

候補単位で起票し、レポート一枚を一カードにしない。現行で解消済み、仕様上必須、効果方向を説明不能、完全重複の案は起票せず理由をレポートへ残す。

## 完了条件

- 選んだObjectiveと観点を明示した。
- 現在のConstraintを必要に応じて整合させた。
- 未被覆候補をEvidence付きINVESTIGATEへ引き渡した。
- 追加計測カード、READY、実装、deploy、benchを行っていない。

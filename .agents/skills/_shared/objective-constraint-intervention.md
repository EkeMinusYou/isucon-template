# Objective / Constraint / Intervention

Backlogは三層で扱う。

- **Objective**: 最終結果を判定する継続的な目的。`SATISFY`、`MAXIMIZE`、`MINIMIZE`を使う。
- **Constraint**: ACTIVE Objectiveを現在制約している、解決策から独立した観測事実。
- **Intervention**: 一体で採否・実装・適用・切り戻す変更。BacklogのB-xxx / B-xxxxカードはすべてInterventionである。IDは最低3桁で4桁にも対応する。

計測、RUN、ログ、プロファイル、コード、設定、公式資料はEvidenceであり、Backlogカードの種類ではない。情報不足や追加計測だけのカードは作らない。

## Objective

開始時に `task backlog -- objective list` でACTIVE Objectiveを確認する。テンプレートの初期値は次のとおり。

- O-001: ベンチと最終整合性チェックを通過する
- O-002: 再起動後の永続性・再現性に関する公式条件を満たす
- O-003: 有効なベンチマークスコアを最大化する

当日の採点仕様に得点要素やpenaltyがある場合は、それぞれを追加Objectiveとして登録する。

Interventionは少なくとも一つのObjectiveへの因果を説明する。ConstraintがなくてもObjectiveへ直接効くInterventionを認める。

## Constraint

Constraintを作るのは、次が揃う場合だけとする。

1. 接続するACTIVE Objective
2. 現在成立しているEvidence
3. 解決策に依存しないidentityとfingerprint
4. 成立snapshotまたはpremise
5. Objectiveを制約する因果
6. resolution conditionまたはreconsider condition

statusは`ACTIVE | RESOLVED | INVALIDATED | MERGED`のみ。制御可能性や探索状態をstatusへ混ぜない。候補がなくても事実が成立する限りACTIVEだが、通常のIntervention探索や実装を止めない。

Interventionとのrelationは次の二つ。

- `RESOLVES`: Intervention単独または一体の依存鎖でresolution conditionを満たす。
- `MITIGATES`: 正方向だが単独ではConstraintを解消しない。

性能Constraintで`RESOLVES`を主張するときだけ、CLIのstructured residual assessmentを使う。`MITIGATES`や非性能Constraintへ同じ数式を強制しない。

## InterventionのREADY条件

カード本文で次の四点を具体化する。専用の巨大な契約JSONは作らない。

1. 目的: どのObjectiveへ、どの因果で効くか
2. 境界: 一体で採否・適用・切り戻す範囲
3. 判定: 何を観測し、どの結果で採用・修正・棄却するか
4. 安全: guardrail、停止条件、rollback

効果量が未知でも、方向・安全性・rollbackを説明できればREADYにできる。実際の挙動を変える実験も通常のInterventionとして扱う。

## 優先順

1. 既存DOING、未検証APPLIED、必要なrollback
2. required-for-valid-resultなObjectiveの回復・保護
3. ユーザー指定
4. ACTIVE ConstraintをRESOLVESするInterventionと未充足依存
5. Objectiveへ直接効くselection、value、loss-recovery
6. MITIGATESと通常の正方向改善

同じ区分ではPriority、依存、Owner、Change boundary競合、完成snapshotを使う。

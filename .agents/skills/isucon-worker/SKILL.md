---
name: isucon-worker
description: READYの実装・正規deployと、手動ベンチ後の採否・修正を担当する。候補探索・計測基盤整備は行わない。
---

# ISUCON worker

実装・レビュー・採否はカード単位で管理し、統合検証・デプロイ・本番確認は複数カードをまとめたdeployment batch単位で行う。1件の実装完了だけではデプロイせず、今回選んだ範囲で[実装手順の区切り条件](references/implementation.md#batchを区切る条件)まで取得可能なREADYを順に処理する。

ベンチ後検証は、指定済みの`Evaluation`に基づく比較要約から採否を判断し、判断に不足する項目だけ追加調査する。対象RUNの共通確認と保存済み検証は再利用する。採用の成立を同方向への追加投資の推奨とせず、局所結果と後続操作の観測を分けて担当へ返す。優先順位の見直しのためにworker自身がTarget更新や新規探索へ広げない。システム全体の診断や、変更と無関係な問題の原因調査は行わない。

## 最初に読む

- `AGENTS.md`
- [Backlog workflow](../../../tools/backlog/backlog-workflow.md)：`Authority`・`Intervention`直下（子節を除く）・`Relations`・`Priority`・`Writer protocol`。ベンチ後は`Adoption`、適用後の棄却時は`Rejection after application`、適用前の棄却時は`Rejection during investigation`、外部待ちは`BLOCKED`、Target達成の確認時は`Target`を追加する。
- [Evidence](../_shared/evidence.md)
- 対象カード、Objective、Target relation、依存、History
- 対象の`docs/official/`

workflowは見出しを検索して必要な節だけ読む。同じ内容を読了済みなら再読しない。実装時は[implementation.md](references/implementation.md)、ベンチ完了・fail・score regression時は[post-benchmark.md](references/post-benchmark.md)に従う。

## 境界

- 実装対象はREADYと、すでに自分が持つDOING・VERIFY・APPLIEDだけ。
- INVESTIGATEから候補を作らず、READY gateを自分で省略しない。
- リモート変更は既存の`task deploy-*`、役割変更は正規のrole収束経路だけを使う。
- Go採用・参考実装の参照専用扱いはAGENTS.mdに従う。生成物の直接編集は禁止する。
- ベンチは実行しない。ユーザーが明示した手動ベンチ結果を検証する。
- 標準計測基盤をInterventionへ混ぜない。新しい計測が必要なら作業を拡大せずユーザーへ報告し、計測の整備・補修は`isucon-setup`へ案内する。

## 優先順

[Work selection](../../../tools/backlog/backlog-workflow.md#work-selection)による今回の着手範囲と根拠を引き継ぎ、その中でworkflowのPriorityに従う。単独呼び出しでは既存カードの寄与仮説・History・保存済みEvidenceから親が範囲を選び、選択のために全体診断や新規探索へ広げない。READYが存在することだけで範囲へ加えず、ユーザーが明示した全件処理と着手済み作業・必要な修正を優先する。

APPLIEDはBacklog全体で同時に最大10件とする。これはCLIではなくworkerが守る運用上の上限である。取得前と適用直前に`task backlog -- list --status APPLIED`でOwnerを問わず件数を確認し、既存APPLIEDと今回のbatchで適用予定のカードの合計を10件以内に収める。同じカードは重複計上しない。合計が10件に達したら新規取得を止め、完成したbatchの検証・適用へ進む。デプロイ後のAPPLIEDが10件を超える場合はデプロイしない。ちょうど10件になる適用は許容する。上限で適用できない場合は既存APPLIEDのベンチ後判定を優先し、手動ベンチ待ちならその旨を報告する。

## 障害復旧

fail、機能エラー、極端なregressionでは、対象RUNのAPPLIED snapshotと直前の実効化変更を先に照合する。対象Interventionに帰属する問題を必要な範囲で修正する。復旧後は正規deployとcorrectness checkまで行う。

## 継続・終了

開始時・各カードの実装レビュー後に継続可否を確認し、今回選んだ範囲で取得可能なら同じbatchへ次のカードを追加する。区切り条件に達したら統合検証・適用を先に行い、その後に共通の[継続・終了手順](../_shared/card-continuation.md)へ進む。ベンチ後の採否判定・修正再適用後も[ベンチ後の継続](references/post-benchmark.md#ベンチ後の継続)に従い、APPLIED枠に空きがあれば選択範囲内の次のREADYへ進む。未評価のAPPLIEDが残っていること自体を、新規取得の停止理由にしない。

ユーザーの停止指示は適用より優先する。対象は、このSkillの担当範囲とユーザーが明示した作業範囲に限り、APPLIED上限を維持する。

## 完了条件

- 実装モード: 今回選んだ範囲でOwner・依存・他作業との競合を確認して取得できるREADYを処理し、各カードをVERIFY、APPLIED、INVESTIGATE、BLOCKED、REJECTEDへ正しく収束した。
- ベンチ後モード: 対象APPLIEDを採用、限定修正、不採用のいずれかへ収束し、APPLIED件数を再確認して継続・終了を判断した。対象RUNの判定完了だけでは終了しない。
- `task backlog -- validate`と関係するローカル・production checkを通した。

完了報告はカードID・状態、変更境界、検証・deploy・採否結果、残るリスクを示す。適用時はdeployment batch数、各batchのカードID・区切った理由・実行Taskと検証結果も示す。適用したカードの改善報告は、各カードの改善仮説・変更境界・Evaluationに沿った指標で、変更内容と観測結果を結びつけて「カード／確認できた改善」の2列で示す。正常性は表外にまとめ、測定前の効果量は未確認とする。具体的な書き方とベンチ後の判定結果は[関連指標の判定結果](references/post-benchmark.md#関連指標の判定結果)に従う。

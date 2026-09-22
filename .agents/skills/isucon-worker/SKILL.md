---
name: isucon-worker
description: READYの実装・正規deployと、verifierからDOINGへ戻された限定修正・不採用の是正を担当する。ベンチ後の採否判断・候補探索・計測基盤整備は行わない。
---

# ISUCON worker

実装・レビューはカード単位で管理し、統合検証・デプロイ・本番確認は複数カードをまとめたdeployment batch単位で行う。1件の実装完了だけではデプロイせず、[実装手順の区切り条件](references/implementation.md#batchを区切る条件)まで取得可能なREADYを順に処理する。

## 最初に読む

- `AGENTS.md`
- [Backlog workflow](../../../tools/backlog/backlog-workflow.md)：`Authority`・`Intervention`直下・`Ownership and handoff`・`Relations`・`Priority`・`Writer protocol`。不採用の是正時は`Rejection after application`、適用前の棄却時は`Rejection during investigation`、外部待ちは`BLOCKED`を追加する。
- [Evidence](../_shared/evidence.md)、対象カード、Objective、Target relation、依存、History
- 対象の`docs/official/`

workflowは見出しを検索して必要な節だけ読む。同じ内容を読了済みなら再読しない。実装・修正・除去は[implementation.md](references/implementation.md)に従う。

## 境界

- 対象は全READY、OwnerなしのDOING、すでに自分が持つDOING・VERIFY。APPLIEDのベンチ後検証と採否は`isucon-verifier`へ渡す。
- INVESTIGATEから候補を作らず、READY gateを自分で省略しない。
- コード・設定の編集と正規deployはworkerが担当する。[カード所有権と計測保護の規則](../../../tools/backlog/backlog-workflow.md#ownership-and-handoff)を守る。
- リモート変更は既存の`task deploy-*`、役割変更は正規のrole収束経路だけを使う。Go採用・参考実装の参照専用扱いはAGENTS.mdに従い、生成物を直接編集しない。
- ベンチは実行しない。新しい計測が必要なら不足を報告し、計測の整備・補修は`isucon-setup`へ案内する。
- Objective・Targetとそのリンクは変更しない。実装上の発見は対象ID・Evidenceとともに担当へ報告する。

## 優先順

自分の未完了DOING・VERIFYと、OwnerなしのDOINGを新規READYより優先する。取得・是正の詳細は[是正手順](references/implementation.md#verifierからの是正)に従う。障害の是正を新規READYの蓄積で遅らせない。

ユーザーが対象を明示的に限定した場合を除き、新着を含む全READYを実装対象とする。Owner・依存・競合を確認して取得可能なものをworkflowのPriorityに従って順に処理する。寄与仮説・History・保存済みEvidenceは処理順の判断に使い、READYを除外する理由にしない。

APPLIEDの件数上限は設けない。未評価のAPPLIEDや修正版の手動ベンチ待ちは、新規取得・batch適用の停止理由にしない。

## 継続・終了

開始時・各カードの実装レビュー後・batch適用後・終了直前に、最新のDOING・VERIFY・READYを確認する。差し戻されたOwnerなしDOINGも取り込み、取得可能な対象があれば継続する。実装レビュー後は同じbatchへの追加を判断し、区切り条件に達したら統合検証・適用を先に行う。その後は共通の[継続・終了手順](../_shared/card-continuation.md)に従う。別スキルを自動起動しない。

ユーザーの停止指示は適用より優先する。対象はこのSkillの担当範囲とユーザーが明示した作業範囲に限る。

## 完了条件

- 取得可能な対象を処理し、実装・是正をVERIFY、APPLIED、INVESTIGATE、BLOCKED、または担当外のREJECTEDへ正しく収束した。適用可能な完成分を残さない。
- 限定修正は再適用後APPLIED、不採用の是正も必要な検証・deploy・本番確認後にrejection-cleanup applicationとしてAPPLIEDへ戻す。いずれもAPPLIEDへの更新時にOwnerを解除し、verifierへ渡す。不採用の最終REJECTED遷移は、再適用後の手動ベンチを確認したverifierが行う。
- `task backlog -- validate`と関係するローカル・production checkを通した。

完了報告はカードID・状態、変更境界と実装内容、検証・deploy結果、残るリスクと未確認事項を示す。適用時はdeployment batch数、各batchのカードID・区切った理由・実行Task・適用IDと検証結果も示す。改善効果の確認・報告と採否判断は、ユーザーによるベンチ実行後にisucon-verifierが担当する。

---
name: isucon-worker
description: ISUCON BacklogのREADY Interventionを実装・検証し、正規Taskfile経路で適用する。手動ベンチ後は対象Interventionを採用、修正、rollbackへ収束させ、障害復旧も同じ採否境界で扱う。新規候補調査や追加計測カード作成には使わない。
---

# ISUCON worker

## 最初に読む

- `AGENTS.md`
- [Backlog workflow](../../../tools/backlog/backlog-workflow.md)：`Authority`・`Intervention`直下（子節を除く）・`Relations`・`Priority`・`Writer protocol`。ベンチ後／棄却時は`VALIDATED and REJECTED`、外部待ちは`BLOCKED`、Constraint更新時は`Constraint`を追加する。
- [Evidence](../_shared/evidence.md)
- 対象カード、Objective、Constraint relation、依存、History
- 対象の`docs/official/`

workflowは見出しを検索して必要な節だけ読む。同じ内容を読了済みなら再読しない。実装時は[implementation.md](references/implementation.md)、ベンチ完了・fail・score regression時は[post-benchmark.md](references/post-benchmark.md)に従う。

## 境界

- 実装対象はREADYと、すでに自分が持つDOING・VERIFY・APPLIEDだけ。
- INVESTIGATEから候補を作らず、READY gateを自分で省略しない。
- リモート変更は既存の`task deploy-*`、役割変更は正規のrole収束経路だけを使う。
- Go採用・参考実装の参照専用扱いはAGENTS.mdに従う。生成物の直接編集は禁止する。
- ベンチは実行しない。ユーザーが明示した手動ベンチ結果を検証する。
- 標準計測基盤をInterventionへ混ぜない。新しい計測が必要なら作業を拡大せずユーザーへ報告する。

## 優先順

workflowのPriorityに従って、担当範囲内の対象を選ぶ。

APPLIEDはBacklog全体で同時に最大10件とする。これはCLIではなくworkerが守る運用上の上限である。新たな適用前に`task backlog -- list --status APPLIED`でOwnerを問わず件数を確認し、適用後も10件以内に収める。10件に達している場合は追加適用を止め、既存APPLIEDのベンチ後判定を優先する。手動ベンチ待ちなら、その旨を報告する。

## 障害復旧

fail、機能エラー、極端なregressionでは、対象RUNのAPPLIED snapshotと直前の実効化変更を先に照合する。根拠のある最小rollbackまたは修正を同じIntervention境界で行い、無関係な変更をまとめて戻さない。復旧後は正規deployとcorrectness checkまで行う。

## 完了条件

- 実装モード: 安全に取得できる対象READYを処理し、各カードをVERIFY、APPLIED、INVESTIGATE、BLOCKED、REJECTEDへ正しく収束した。
- ベンチ後モード: 対象APPLIEDを採用、限定修正、rollbackのいずれかへ収束した。
- `task backlog -- validate`と関係するローカル・production checkを通した。

完了報告はカードID・状態、変更境界、検証・deploy・rollback結果、残るリスクに絞る。

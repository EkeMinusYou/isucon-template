---
name: isucon-worker
description: ISUCON BacklogのREADY Interventionを実装・検証し、正規Taskfile経路で適用する。手動ベンチ後は対象Interventionを採用、修正、rollbackへ収束させ、障害復旧も同じ採否境界で扱う。新規候補調査や追加計測カード作成には使わない。
---

# ISUCON worker

READY Interventionを安全に実装・適用し、手動ベンチ後の採否まで収束させる。

## 最初に読む

- `AGENTS.md`
- [Objective / Constraint / Intervention](../_shared/objective-constraint-intervention.md)
- [Evidence](../_shared/evidence.md)
- 対象カード、Objective、Constraint relation、依存、History
- 対象の`docs/official/`

実装時は[implementation.md](references/implementation.md)、ユーザーがベンチ完了・fail・score regressionを伝えた場合は[post-benchmark.md](references/post-benchmark.md)も読む。

## 境界

- 実装対象はREADYと、すでに自分が持つDOING・VERIFY・APPLIEDだけ。
- INVESTIGATEから候補を作らず、READY gateを自分で省略しない。
- リモート変更は既存の`task deploy-*`、役割変更は正規のrole収束経路だけを使う。
- アプリの編集対象はGo実装とする。`webapp/node/`を含む他言語の参考実装は参照専用とし、編集は禁止する。生成物、サーバー上の直接編集も禁止する。
- ベンチは実行しない。ユーザーが明示した手動ベンチ結果を検証する。
- 標準計測基盤をInterventionへ混ぜない。新しい計測が必要なら作業を拡大せずユーザーへ報告する。

## 優先順

共有規律に従う。まず既存DOING・未検証APPLIED・rollback、次にvalidity、ユーザー指定、RESOLVESと依存、Objective直結、MITIGATESを扱う。Owner、dependency、dirty diff、完成snapshotの整合を優先順より先に守る。

APPLIEDはBacklog全体で同時に最大10件とする。これはCLIではなくworkerが守る運用上の上限である。新たな適用前に`task backlog -- list --status APPLIED`でOwnerを問わず件数を確認し、適用後も10件以内に収める。10件に達している場合は追加適用を止め、既存APPLIEDのベンチ後判定を優先する。手動ベンチ待ちなら、その旨を報告する。

## 状態遷移

- claim: `READY -> DOING`をOwner設定と同じversion-checked updateで行う。
- 実装とローカル検証完了: `DOING -> VERIFY`
- 正規deployとproduction状態確認完了: `VERIFY -> APPLIED`
- 手動ベンチ後に採用: `task pass`
- 修正が必要: 同じ採否境界なら`APPLIED/VERIFY -> DOING`
- 技術的反証または安全に成立しない: rollback後`REJECTED`
- 境界・因果の再調査が必要: Ownerを外して`INVESTIGATE`

BLOCKEDは具体的な外部待ちだけに使い、問い、取得経路、resume triggerを残す。

## 障害復旧

fail、機能エラー、極端なregressionでは、対象RUNのAPPLIED snapshotと直前の実効化変更を先に照合する。根拠のある最小rollbackまたは修正を同じIntervention境界で行い、無関係な変更をまとめて戻さない。復旧後は正規deployとcorrectness checkまで行う。

## 完了条件

- 実装モード: 安全に取得できる対象READYを処理し、各カードをVERIFY、APPLIED、INVESTIGATE、BLOCKED、REJECTEDへ正しく収束した。
- ベンチ後モード: 対象APPLIEDを採用、限定修正、rollbackのいずれかへ収束した。
- `task backlog -- validate`と関係するローカル・production checkを通した。

完了時は、カードIDと状態、変更境界、検証、deploy Task、rollback有無、残るリスク、ベンチを自分では実行していないことを報告する。

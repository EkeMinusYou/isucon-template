---
name: isucon-setup
description: ISUCON競技開始時に、公式資料と実環境からリポジトリを取得し、Taskfileの役割定義、正規deploy、標準bench・Evidence回収経路を利用可能にする。性能改善の実装や候補起票には使わない。
---

# ISUCON setup

## 最初に読む

- `AGENTS.md`
- `docs/official/`の当日マニュアルとアプリケーション仕様
- [`tools/README.md`](../../../tools/README.md)の競技開始時設定手順とディレクトリ別チェックリスト
- [Backlog workflow](../../../tools/backlog/backlog-workflow.md)の`Objective`・`Writer protocol`節のみ（Objective更新時）。

参照文書は見出しを検索して必要な節だけ読む。同じ内容を読了済みなら再読しない。

## 作業境界

- Go採用、参考実装の参照専用扱い、生成物、クロスコンパイル、正規setup/deployはAGENTS.mdに従う。
- 既存の正規deploy経路で扱える対象に専用タスクを増やさない。
- 最適化、Constraint作成、Intervention起票は行わない。

## 手順

1. 公式資料から変更可能範囲、初期化、整合性、再起動、最終追試条件を確認する。
2. `task inspect-hosts`などの読み取り専用SSHでホスト、CPU・メモリ、private IPに加え、稼働・enable済みservice、unit、process、listen portを確認し、固定のservice一覧だけで探索を終えない。
3. 競技の動作・初期化・採点とその依存serviceを、標準外も含めすべて分類する。対象外は公式資料・実環境に基づく理由を残し、管理対象のアプリ・設定・unit・schemaは`task setup-*`で取得してgit管理する。
4. `Taskfile.yml`の役割とIPを実環境へ合わせ、標準外serviceのroleとservice名も正本へ追加して、`task gen`で生成物を作る。
5. `tools/README.md`のチェックリストに従い、全管理対象serviceを正規のsetup、deploy、role収束、状態・疎通検査、collector、digester、分析、RUNの役割記録へ組み込む。適用しない項目は理由を残す。必須adapterを実環境に合わせ、未使用のoptional機能は無効にする。
6. schemaの取得先と設定構文検査方法をTaskfileへ明示し、`task setup-check`を通す。
7. `task deploy-app-dry`と`task deploy-all-dry`で転送先、role、activation順を確認する。
8. 正規の`task deploy-*`で反映し、必要なら`task apply-roles`を行った後、`task check-roles`と`task check-network`を行う。
9. 対象と影響を確認したうえで`task before-bench`／`task abort-run`を使い、RUN採番・collector起動・中断時の掃除を確認する。
10. `task artifacts`で生成物と読み手の整合、baseline RUNの`task artifacts-run`で成果物と欠損検出を検査する。同じRUN IDへ成果物と`run.json`が揃い、完全欠損は`missing`と検査失敗になることを確認する。collectorあり／なしの対になるRUNで計測負荷を確認する。
11. `task backlog -- objective list`で初期Objectiveを確認し、当日の採点仕様に必要なObjectiveを追加する。

このテンプレートはDB初期化Taskを定義しない。公式手順に従って追加する場合は、通常deployと分離し、
ユーザーの依頼範囲、対象、復旧方法を確認してから実行する。

## 完了条件

全管理対象ホストへの正規deployと、上記の役割・疎通・成果物検査が通り、当日のObjectiveをCLIで参照できる。
完了報告は構成・検査結果・未確認事項を中心に、service分類と対象外／非適用理由、その根拠となる公式資料を示す。

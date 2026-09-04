---
name: isucon-setup
description: ISUCON競技開始時に、公式資料と実環境からリポジトリを取得し、Taskfileの役割定義、正規deploy、標準bench・Evidence回収経路を利用可能にする。性能改善の実装や候補起票には使わない。
---

# ISUCON setup

競技開始時の環境を、リポジトリから再現可能なdeploy・bench状態へ収束させる。

## 最初に読む

- `AGENTS.md`
- `docs/official/`の当日マニュアルとアプリケーション仕様
- [`tools/README.md`](../../../tools/README.md)の競技開始時設定手順とディレクトリ別チェックリスト
- [Objective / Constraint / Intervention](../_shared/objective-constraint-intervention.md)

公式資料と過去メモが違う場合は公式資料を優先する。

## 作業境界

- サーバー上で直接編集せず、`task setup-*`で取得してgit管理し、`task deploy-*`で反映する。
- 役割の正本は`Taskfile.yml`冒頭の役割変数とIP mapだけとする。
- `nginx/conf.d/upstream.conf`は`task gen`の生成物なので手編集しない。
- Goは読み取り専用確認で得た実ホストのarchitectureへローカルでクロスコンパイルする。
- 参考実装が複数ある場合、採用言語以外は仕様参照専用とし、正規deploy対象へ混ぜない。
- 既存の正規deploy経路で扱える対象に専用タスクを増やさない。
- 最適化、Constraint作成、Intervention起票は行わない。

## 手順

1. 公式資料から変更可能範囲、初期化、整合性、再起動、最終追試条件を確認する。
2. `task inspect-hosts`などの読み取り専用SSHでホスト、CPU・メモリ、稼働service、unit、設定、private IPを確認する。
3. `task setup-*`でアプリ・設定・schemaをローカルへ取得する。
4. `Taskfile.yml`の役割とIPを実環境へ合わせ、`task gen`で生成物を作る。
5. `tools/README.md`のディレクトリ別チェックリストに従い、deploy、collector、digester、分析、アプリ固有adapterを確認する。
6. schemaの取得先と設定構文検査方法をTaskfileへ明示し、`task setup-check`を通す。
7. `task deploy-app-dry`と`task deploy-all-dry`で転送先、role、activation順を確認する。
8. 正規の`task deploy-*`で反映し、必要なら`task apply-roles`を行った後、`task check-roles`と`task check-network`を行う。
9. 対象と影響を確認したうえで`task before-bench`／`task abort-run`を使い、RUN採番・collector・digest・manifest経路を確認する。
10. baseline RUNを`task artifacts-run`で検査し、collectorあり／なしの対になるRUNで計測負荷を確認する。
11. `task backlog -- objective list`で初期Objectiveを確認し、当日の採点仕様に必要なObjectiveを追加する。

このテンプレートはDB初期化Taskを定義しない。公式手順に従って追加する場合は、通常deployと分離し、
ユーザーの依頼範囲、対象、復旧方法を確認してから実行する。

## 完了条件

- リポジトリの変更から全必要ホストへ正規deployできる。
- service状態と役割がTaskfileの定義へ収束する。
- 標準benchサイクルが同じRUN IDへ成果物を集め、`run.json`へ状態を記録できる。
- 生成物と読み手の整合を`task artifacts`で確認できる。
- 実RUNの完全欠損を`run.json`の`missing`と`task artifacts-run`の失敗で検出できる。
- `tools/README.md`の必須adapterを実環境に合わせ、使わないoptional機能を明示的に無効のままにしている。
- baseline RUNでcollector負荷と成果物の欠損を確認できる。
- 初期Objectiveと、当日の採点仕様から追加したObjectiveをCLIで参照できる。

完了時は、確認した公式資料、役割構成、実行したsetup/deploy/check、未確認事項を簡潔に報告する。

---
name: isucon-setup
description: ISUCON競技開始時に、公式資料と実環境からリポジトリを取得し、Taskfileの役割定義、正規deploy、標準bench・Evidence回収経路を利用可能にする。性能改善の実装や候補起票には使わない。
---

# ISUCON setup

## 最初に読む

- `AGENTS.md`
- `docs/official/`の当日マニュアルとアプリケーション仕様
- [`tools/README.md`](../../../tools/README.md)の基本方針・ディレクトリ別チェックリスト・Taskfile・検証コマンドの意味。他の設定項目は対応する節だけ読む。
- [Backlog workflow](../../../tools/backlog/backlog-workflow.md)の`Objective`・`Writer protocol`節のみ（Objective更新時）。

参照文書は見出しを検索して必要な節だけ読む。同じ内容を読了済みなら再読しない。

## 作業境界

- Go採用、参考実装の参照専用扱い、生成物、クロスコンパイル、正規setup/deployはAGENTS.mdに従う。
- 既存の正規deploy経路で扱える対象に専用タスクを増やさない。
- 最適化、Constraint作成、Intervention起票は行わない。

## 進め方

- 最初に作業ディレクトリ、git差分、使用するTaskfile、対象ホスト、ローカルの進行中RUNとリモートcollectorのRUN IDを照合する。複数の作業環境から同じ競技サーバーを操作する場合、別環境の計測については、所有元と終了状態が確認できるまで停止・ログローテート・deployを進めない。残存の表示だけで`abort-run`しない。
- ベンチの実行担当はユーザーの指示に従う。手動実行の場合は計測・回収経路を整え、実地確認が必要になった時点で目的と手順を伝える。ベンチコマンドの聴取や自動実行経路の新設をsetupの前提にしない。setupの依頼だけでベンチ実行まで承認されたと扱わない。
- 当日の動作、正規deploy、必須成果物の回収を妨げる問題を優先する。汎用基盤の拡張、optional計測、分析UIの追加へ作業を広げない。完了に必要な不具合修正は行い、追加の改善案は未実施として記録する。

- 調査は一覧から詳細へ進める。資料は見出し、実環境はservice名・状態・process・待受ポートから対象を把握し、競技との関係があるものや関係が不明なものの設定・ログを絞って読む。大量の調査結果は`raw/`へ保存して必要な範囲だけ表示する。出力が省略された部分は未確認として範囲を絞り直し、固定のservice一覧だけで探索を終えない。
- 構成は実環境の配置を基準にする。役割や通信先を変更する前に、確認した現在の配置、採用する配置とsetup上の理由、対象ホストと影響を短くユーザーへ説明する。稼働ホストと通信先ホストを分ける場合はその理由も示す。この説明を新たな承認待ちにはしない。既存の権限・承認条件に従う。
- 検証は変更箇所の構文検査・build・必要なテスト、対象deployのdry-run、`task setup-check`による全体確認の順に進める。失敗後は原因に関係する個別検証から再開し、通過済みの検証は新しい変更や未解決の懸念がある場合に再実行する。全体確認に含まれる検査を、手順の記載だけを理由に直後に繰り返さない。

## 手順

1. 公式資料から変更可能範囲、初期化、整合性、再起動、最終追試条件を確認する。
2. `task inspect-hosts`などの読み取り専用SSHでホスト、CPU・メモリ、private IPに加え、稼働・enable済みservice、unit、process、listen portを確認し、固定のservice一覧だけで探索を終えない。
3. 競技の動作・初期化・採点とその依存serviceを、標準外も含めすべて分類する。対象外は公式資料・実環境に基づく理由を残し、管理対象のアプリ・設定・unit・schemaは`task setup-*`で取得してgit管理する。
4. `Taskfile.yml`の役割とIPを実環境へ合わせ、標準外serviceのroleとservice名も正本へ追加して、`task gen`で生成物を作る。
5. `tools/README.md`のチェックリストに従い、全管理対象serviceを正規のsetup、deploy、role収束、状態・疎通検査、collector、digester、分析、RUNの役割記録へ組み込む。適用しない項目は理由を残す。必須adapterを実環境に合わせ、未使用のoptional機能は無効にする。
6. schemaの取得先と設定構文検査方法をTaskfileへ明示し、変更箇所の個別検証と対象deployのdry-runを通す。
7. `task setup-check`で全体確認する。内包する`task deploy-app-dry`と`task deploy-all-dry`の結果から転送先、role、activation順を確認する。
8. 正規の`task deploy-*`で反映し、必要なら`task apply-roles`を行った後、`task check-roles`と`task check-network`を行う。
9. 複数の作業環境から同じ競技サーバーを操作する場合は、サーバーの使用状況と対象RUNの所有元を照合する。対象と影響を確認したうえで`task before-bench`／`task abort-run`を使い、RUN採番・collector起動・中断時の掃除を確認する。
10. `task artifacts`で生成物と読み手の整合を検査する。baselineが必要になったら実行担当へ依頼し、完了したRUNの`task artifacts-run`で成果物と欠損検出を検査する。同じRUN IDへ成果物と`run.json`が揃い、完全欠損は`missing`と検査失敗になることを確認する。その後、collectorあり／なしの対になるRUNで計測負荷を確認する。結果待ちの間は独立した必要作業を進める。
11. `task backlog -- objective list`で初期Objectiveを確認し、当日の採点仕様に必要なObjectiveを追加する。

このテンプレートはDB初期化Taskを定義しない。公式手順に従って追加する場合は、通常deployと分離し、
ユーザーの依頼範囲、対象、復旧方法を確認してから実行する。

## 完了条件

全管理対象ホストへの正規deployと、上記の役割・疎通・成果物検査が通り、当日のObjectiveをCLIで参照できる。
完了報告は構成・検査結果・未確認事項を中心に、service分類と対象外／非適用理由、その根拠となる公式資料を示す。手動ベンチの結果待ちは、環境の反映・疎通確認とRUNによる検証の完了状態を分けて伝え、必要な実行を依頼する。時間を理由に必須作業を打ち切ったり、未確認のまま完了と報告したりしない。

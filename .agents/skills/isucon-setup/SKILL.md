---
name: isucon-setup
description: ISUCONの初期環境と標準計測を整備し、既存環境の計測不足も補修する。取得・正規deployからpprof、fgprof、nginxログ、user-transitionの回収・分析・dashboard表示まで扱う。性能改善の実装・候補起票には使わない。
---

# ISUCON setup

初期環境の構築と、標準計測を分析に使える状態にする作業を担当する。
既存環境の計測補修では、指定範囲とその生成・回収・読み手だけを調べ、初期構築全体を繰り返さない。
調査のみの依頼なら調査結果まで、整備の依頼なら必要なローカル修正・正規deploy・検証まで進める。

## 最初に読む

- `AGENTS.md`と`docs/official/`の関連資料。公式資料を優先する。
- [Tools](../../../tools/README.md)の基本方針と対象項目のチェックリスト。
- 計測を扱うときは[標準計測の確認手順](references/measurement.md)、Taskfileの役割・計測変数、collector/digester宣言。
- Objective更新時のみ[Backlog workflow](../../../tools/backlog/backlog-workflow.md)のObjective・Writer protocol節。

## 作業境界

- Go採用、参考実装の参照専用扱い、生成物、クロスコンパイル、正規setup/deployはAGENTS.mdに従う。
- ベンチはユーザーが実行する。`task bench*`を含め代理実行しない。setup依頼をベンチ実行許可とみなさない。
- 作業環境・Taskfile・対象ホスト・進行中RUNの所有元を照合し、別環境を含め稼働中RUNへrestart・ローテート・計測条件変更を割り込ませない。
- 最適化、Constraint・Interventionの起票は行わない。計測整備はBacklogとは独立して扱う。
- 既存の正規deploy経路を使う。DB初期化は通常deployと分離し、依頼範囲・対象・復旧方法を確認する。
- 計測負荷比較と再起動後のスコア再現性検証は初期setupの完了条件に含めない。明示依頼時は別の検証範囲として扱う。

## 標準計測

以下は初回setupの必須対象である。未実装・未設定を理由にoptionalへ落とさない。

- Go CPU・heap・allocs・goroutineのpprof、fgprof、採取開始確認endpoint。
- nginxアクセスログの必要列、ローテート、全NGINX_HOSTSの回収、alp・upstream集計。
- user-transitionの識別方法・ログ列・アプリ固有API分類・集計。
- DB・ホスト・全管理対象serviceと依存サービスの標準メトリクス、app/nginx・kernel/OOM等の障害ログ。
- 通常ベンチへの自動収集接続、ホスト別の必須成果物・内容・時間窓の検査、対応するDuckDB・dashboardの読み手。

有効対象の正本はcollector/digesterの`enabled_by_default`。profileは`group: profiles`の有効な宣言を
自動収集・開始確認・必須成果物判定で共通利用する。`PROFILES_ENABLED=false`は比較等での明示的な停止用であり、
未設定を隠すために使わない。宣言と自動実行対象の名前一覧を別々に維持しない。

仕様上成立しない計測は根拠を示して対象外にできる。たとえば安定したセッションを識別できないアプリでの
user-transitionは、代替の識別方法も確認したうえで理由を記録する。高頻度task-state・lock wait、
nginx on-CPU、block/mutex、アプリ内部状態などの追加計測は目的・負荷・停止方法を確認して選ぶ。

## 初回セットアップ

1. 公式資料から構成・変更可能範囲・初期化・採点・追試条件を確認する。
2. 読み取り専用SSHで全ホストの資源、IP、service・unit・process・listen portと依存経路を確認する。
   管理対象のアプリ・設定・unit・schemaを`task setup-*`で取得し、対象外は理由を残す。
3. Taskfileの役割・IP・service名・TARGET_OS/ARCH・schema取得先・構文検査を実環境へ合わせ、`task gen`と`task setup-check`を通す。
4. 全管理対象を正規deploy・role収束・計測・分析・RUNの役割記録へ組み込む。標準計測のadapterと通常ベンチの起動・回収待ちも整える。
5. 対応する`task deploy-*-dry`で転送先とactivationを確認して正規deployし、`task check-roles`・`task check-network`と計測の疎通を確認する。
6. `task artifacts`と短いログ・profile取得で生成から読み手までを確認する。collectorの起動・停止・timeout・欠損検出も確認する。
7. ユーザーが実行したbaseline RUNで`task artifacts-run`、内容・ホスト・時間窓、DuckDB・dashboard表示を確認する。
   通常の回収は失敗RUNも`after-bench`でfinalizeする。自動ローカルcommitの副作用を確認し、`abort-run`を通常回収の代用にしない。
8. 当日の採点仕様に必要な初期ObjectiveをCLIで確認・整備する。計測のためのカードは作らない。

## 既存環境の計測補修

1. 対象RUNと実効設定を照合し、生成・起動条件・回収・形式・時刻・分析・画面のどこで不足したかを切り分ける。
2. 対象ホスト、変更理由、元の値、停止・切り戻し方法を定め、出力側と回収・検査・読み手を一緒に修正する。
3. 変更に対応するテスト・構文検査・buildを行い、サーバー側変更があれば正規deployと疎通確認まで行う。
4. 保存済みRUNは上書きしない。実負荷の再確認が必要ならユーザーへ次RUNを依頼し、未検証範囲を明示する。

## 完了条件と報告

計測ごとに対象ホスト・生成元・有効条件・回収先・読み手・根拠を記録し、状態を
「実負荷で確認済み」「疎通確認済み・ベンチ待ち」「未設定」「取得失敗」「意図的に無効」「対象外」に分ける。
無効・対象外には理由を添える。初回setupでは標準計測全件、補修では依頼範囲について報告する。

ファイルの存在やヘッダーだけのfallback、HTTP 200だけで完了としない。対応する分析・dashboardまで確認する。
baseline待ちは「設定・疎通確認済み、実負荷検証待ち」とし、全計測成立とは報告しない。
必須項目が未設定・取得失敗なら、その範囲のsetupは未完了。

RUNに紐づく報告は[命名規則](../../../docs/reports/README.md)に従い`docs/reports/isucon-setup/`へ新規保存する。
RUNがなければ完了報告へ残し、架空のRUNは作らない。過去の別skill名のレポートは履歴として保持する。

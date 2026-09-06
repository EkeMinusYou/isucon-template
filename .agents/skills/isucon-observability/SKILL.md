---
name: isucon-observability
description: ISUCONの計測不足を調査し、nginxアクセスログ、Go pprof・fgprof、DB・ホスト・依存サービスの計測を正規の収集・集計経路へ整備して検証する。Backlogから独立した計測基盤作業として使い、性能改善の起票やベンチ実行は行わない。
---

# ISUCON observability

計測の生成から回収・集計・解析までをつなぎ、同じRUNと時間窓で使えるEvidenceを整える。
初期環境の取得・構成は`isucon-setup`、既存環境の計測不足の修正と計測負荷の評価はこのスキルが担当する。

## 最初に読む

- `AGENTS.md`と対象に関係する`docs/official/`
- [Tools](../../../tools/README.md)の基本方針、計測と集計、access log、分析。その他の節は対象に応じて読む。
- [Evidence](../_shared/evidence.md)と[レポート命名規則](../../../docs/reports/README.md)
- `Taskfile.yml`の役割・計測変数と対象タスク、`tools/measurectl/collectors.yaml`、`digesters.yaml`

## 境界

- Backlogとは独立したリポジトリ作業として調査・実装・適用・検証する。計測整備のためにObjective・Constraint・Interventionを作成・更新せず、investigateやworkerへの引き渡しも行わない。
- 調査のみの依頼なら不足と修正案の報告までに留める。整備の依頼なら必要な修正と正規deployまで進める。性能改善、業務ロジックの変更、配置変更を混ぜない。
- ベンチマーカーは実行しない。ラッパーの`task bench*`も実行せず、必要な負荷走行はユーザーへ依頼する。インターネット調査、サーバー編集、生成物、Go採用、他環境のRUN保護はAGENTS.mdに従う。
- 進行中RUNに設定変更・restart・ローテートを割り込ませない。計測条件の変更は次のRUNから適用し、既存RUNの成果物・manifestを新しい条件で上書きしない。

## 不足を調べる

1. 作業ディレクトリ、git差分、Taskfile、対象ホスト、進行中RUNの所有元を確認する。指定RUN、指定なしなら最新のfinalized RUNの`run.json`で役割・source・load window・artifact statusを読む。RUNがない場合はコード・設定と読み取り専用SSHで進め、負荷中の確認は未検証とする。
2. [確認項目](references/coverage.md)に沿って、入口からGoアプリ、DB、その他の依存サービス、ホストまで確認する。対象指定があればその範囲を優先し、関連する不足も記録する。
3. 項目ごとに「対象ホスト・service／知りたい事実／生成元／有効条件／回収先／読み手／状態／根拠」を整理する。状態は取得確認済み・欠損・無効・対象外・未確認を区別する。設定の存在や空のTSVだけでは取得確認済みにしない。対象外・無効には理由を残す。
4. 欠損を出力設定、endpoint登録、到達性・権限、起動条件、ローテート、回収、形式・集計、時間窓のどこで生じたか切り分ける。必要な標準計測の修復を優先し、optional計測は解きたい問いがあるものだけ選ぶ。

## 修正して検証する

1. 各修正の対象ファイル・ホスト、元の値、採用理由、停止・切り戻し方法を明確にする。既存の設定・引数で対応し、必要な場合だけcollectorやアプリの計測endpointを拡張する。未取得の設定は`task setup-*`で取得するが、既存のローカル変更を上書きしない。
2. 出力側と回収側を一緒に直す。成果物を変更する場合は宣言・manifest契約・digester・分析schemaと読み手を揃え、必要ならdashboardも更新する。生成・回収に失敗した必須成果物をoptional化して検査を通さない。
3. 変更に対応する構文検査、Goのbuild・必要なテスト、`task artifacts`を行う。collectorの変更では開始・停止・timeout・失敗時の欠損を、parserの変更では実際のログ形式と空・不正入力を検証する。検査の重複はToolsの方針に従う。
4. 対応する`task deploy-*-dry`でホスト・転送先・activationを確認して正規deployし、実効設定、service状態、出力と読み手の整合を確認する。短い疎通・profile取得の確認と、負荷中の計測成立は分ける。
5. 負荷中の検証が必要なら、ユーザーへベンチ実行を依頼し、対象RUN・必要な計測・取得タイミングを伝える。所有元を確認したRUNで正規の`before-bench`／profile取得／`after-bench`を使う。通常の回収はfinalizeし、`abort-run`を回収の代用にしない。実行前に`after-bench`の自動ローカルコミットを含む現行タスクの副作用を確認する。
6. 保存済みRUNに`task artifacts-run RUN=runs/<RUN_ID>`を実行し、内容・ホスト・時間窓・profileの解析可能性まで確認する。ユーザーの結果待ちなら、修正・疎通確認済みの範囲と負荷中の未検証項目を報告する。
7. 計測負荷は同じアプリ・配置・負荷条件で比較する。collectorあり／なしのRUNはユーザーに依頼し、scoreだけでなくCPU時間・メモリ・I/O・待ちも比較する。ログやprofileが比較モードで本当に停止するか確認し、周期collectorなしの結果だけで全計測の負荷を評価しない。高頻度・optional計測は負荷未評価のまま常時有効化しない。

## 完了報告

計測の対応表、変更と切り戻し方法、検査結果、参照RUN、取得確認済み／未検証／対象外、計測負荷の判断を報告する。ベンチ待ちや取得失敗を計測成立と扱わない。確認できた不足だけでなく、調査できなかった範囲も明示する。

RUNに紐づく報告は命名規則に従い`docs/reports/isucon-observability/`へ新規保存する。RUNがない場合は完了報告へ残し、架空のRUN IDを作らない。

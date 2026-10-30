# ISUCON計測ダッシュボード

保存済みRUNを読むローカルのReact UIです。競技アプリへ組み込まず、競技サーバーのサービス操作も行いません。

リポジトリのルートで`task dashboard`を実行すると、APIが`127.0.0.1:8091`、UIが`127.0.0.1:5173`で起動します。Node.js・npm・Goと、分析DBを読むDuckDBが必要です。初回はUIの依存を自動インストールします。

## 入力と表示

| 表示 | 入力 |
|---|---|
| サーバーの役割 | 各RUNの`run.json`。現在のTaskfileではなく、記録当時のGo・HTTP入口・nginx・追加サービスを表示 |
| スコア・合否 | `runs/scores.tsv`、`run.json`、分析DBの採点結果 |
| タイムライン・alp・操作遷移 | Goの`raw/access-*.log[.zst]`とRUNの集計成果物・ベンチ出力 |
| ホスト・サービス資源 | `*-proc-metrics.tsv`、`*-service-metrics.tsv`。Goと追加サービスをservice名で区別。過去RUNのnginxも表示可能 |
| Go runtime | `*-runtime-metrics.tsv`。goroutine・heap・allocation速度・GCのCPU時間 |
| Go pprof・任意fgprof | RUNに保存されたprofile。採取期間全体の値であり、初期化を含む場合がある |

Go runtimeは、分析DBに有効な負荷期間があればそのサンプルだけを表示します。期間が未確定なら採取全体であることを画面に示します。未取得値は0にせず、counterの速度は同じプロセスの隣接サンプル間で計算します。再起動やcounter減少をまたぐ速度は表示しません。

旧RUNにruntimeや役割がなくても、存在する他の成果物を表示できます。標準Goロガーのcounterは`access_log_health`で確認します。過去のnginxログへこのcounterを補いません。アプリ固有の操作定義・セッション識別値は、bootstrapで接続してから確認してください。

## 計測からの接続

`task bench`が計測開始・ベンチ・回収・分析DB同期を行います。保存済み成果物からDBだけを更新する場合は`task q-sync`を使います。生成元と回収の契約は[計測ガイド](../../../docs/measurement/README.md)、[共通ツール](../../README.md)を参照してください。

## ローカル検証

このディレクトリで`npm ci`、`npm test`、`npm run build`を実行します。APIの検証は`go -C ../server test ./...`です。画面確認は[ブラウザツール](../../browser/README.md)を利用できます。

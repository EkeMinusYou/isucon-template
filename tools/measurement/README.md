# 共通計測module

アプリ内の計測とホスト上のcollectorを一つのGo moduleで管理する。
アプリは`app`パッケージだけをimportする。proc・MySQLなどのcollectorは独立した実行ファイルであり、
アプリの起動・停止とは別にTaskfileとmeasurectlが配布・実行・回収する。
分析は保存したpprof・TSV・JSONを通じて接続し、DuckDB・dashboardのコードはこのmoduleへ含めない。

| パス | 内容 |
|---|---|
| `app/` | CPU・任意fgprof・2秒ごとのheap/goroutine、RUN別開始確認・停止通知、1秒のruntime・SQL pool・内部指標の公開 |
| `cmd/proc-metrics/` | Linuxのホスト・service・disk・blocked task収集 |
| `cmd/mysql-metrics/` | MySQL status・接続・lock wait収集 |
| `cmd/sql-pool-metrics/` | アプリのSQL pool・内部指標をHTTPから収集 |
| `cmd/nginx-oncpu-profiler/` | optional nginx workerのon-CPU profile収集（perfを使用） |
| `cmd/json-metrics-collector/` | 任意のenable・snapshot・disable endpointから追加指標を収集 |

## アプリへの導入

リポジトリ内の共通moduleを使う場合は、アプリのgo.modに次を追加する。
`replace`の相対パスはアプリのgo.modから計測moduleのディレクトリまでを指定する。
このリポジトリでは`webapp/go`から`../../tools/measurement`となる。

```go
require github.com/EkeMinusYou/isucon-template/tools/measurement v0.0.0

replace github.com/EkeMinusYou/isucon-template/tools/measurement => ../../tools/measurement
```

DBの接続が完成した後、アプリ起動時に一度だけ呼ぶ。

```go
import "github.com/EkeMinusYou/isucon-template/tools/measurement/app"

server, err := app.Start(app.Options{
    Pools: []app.Pool{{
        DB: db, DatabaseHost: "db.local", Name: "main", Role: "primary",
    }},
})
if err != nil {
    log.Fatal(err)
}
defer server.Close()
```

`DB`は`Stats() sql.DBStats`を持つ値（`*sql.DB`・`*sqlx.DB`など）。接続先・pool名・role・shardは
競技アプリの構成に合わせた固定ラベルを指定する。DBが複数なら`Pools`へ追加する。
profileだけが必要なプロセスでは`Pools`を省略できる。
既定listenerはloopbackの6060番。bind失敗は同期的に返るため起動時に扱う。
副作用のある`init()`は使わず、別プロセスモードを持つアプリでも必要なモードだけで開始する。

任意の内部指標は`Options.Snapshot`へcallbackを設定する。

```go
Snapshot: func() app.Snapshot {
    return app.Snapshot{
        Metrics: map[string]any{"events_total": counter.Load()},
    }
},
```

callbackはHTTP要求ごとに呼ばれるので、並行アクセスに対応し、短時間で返す。
`Metrics`は有限の数値だけを含み、名前・項目数は[SQL poolの契約](docs/sql-pool-metrics.md)に合わせる。
`Extra`は独自のJSONフィールドを追加する用途で、標準フィールドを上書きできない。
アプリ固有のWAL・queueの意味、認証・セッション識別はアプリ側に残す。

moduleは現在このリポジトリのローカル参照で利用する。外部への公開・バージョンのタグ付けは行っていない。

## ビルドと検証

操作の入口はルートのTaskfile。

```sh
task build-measurement
task test-measurement
task build
```

collectorは既存の`TARGET_OS`・`TARGET_ARCH`でクロスコンパイルする。
既存の各`build-*-metrics`タスクも利用できる。
collector宣言のbinaryパスは`tools/measurement/cmd/<name>/<name>`で、
起動引数・remote directory・回収先ファイル名・TSV列は移動前と同じ。
MySQL collectorの`-dsn`は必須で、競技固有の認証情報をmoduleの既定値に持たない。
Taskfileの既存設定はすでに`-dsn`を渡している。
optional JSON collectorのendpoint契約は[こちら](docs/json-metrics-collector.md)。

標準計測の導入では、このmoduleをビルドしてDB pool・service・接続先を設定する。
nginxログ形式、MySQL設定、SSH・systemdの設定はTaskfileと既存のsetup/deployで整える。
Go依存のダウンロード・コンパイルやサーバー設定の時間は引き続き必要。

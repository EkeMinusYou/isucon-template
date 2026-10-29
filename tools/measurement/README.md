# 共通計測module

Goアプリの計測と独立したGo collectorを管理します。アプリは`app`パッケージをimportし、分析ツールは依存へ含めません。

| パス | 内容 |
|---|---|
| `app/` | CPU、任意fgprof、2秒ごとのheap/goroutine、runtime・内部指標、開始確認・停止、要求body計測・ログ出力counter |
| `cmd/proc-metrics/` | Linuxホスト・Goサービス・disk・任意のblocked task |
| `cmd/runtime-metrics/` | `/debug/runtime`を1秒ごとに取得 |
| `cmd/json-metrics-collector/` | 任意のenable・snapshot・disable endpointから追加指標を取得 |

## アプリへ接続する

テンプレートの`webapp/go/go.mod`・`server.go`は接続済みです。起動方法を置き換える場合も同じmoduleを使います。別moduleからは次の参照を追加します。

```go
require github.com/EkeMinusYou/isucon-template/tools/measurement v0.0.0
replace github.com/EkeMinusYou/isucon-template/tools/measurement => ../../tools/measurement
```

アプリ起動時に一度呼びます。既定はloopbackの6060番で、bind失敗は同期的に返します。

```go
server, err := app.Start(app.Options{})
if err != nil { log.Fatal(err) }
defer server.Close()
```

`Options.Snapshot`へ競技固有の内部指標を追加できます。

```go
Snapshot: func() app.Snapshot {
    return app.Snapshot{Metrics: map[string]any{"events_total": counter.Load()}}
},
```

callbackは並行アクセスに対応し、短時間で返してください。名前は`[a-z][a-z0-9_]{0,31}`、値は有限の数値に限ります。既定では1回1024項目、応答1MiB、TSV全体32MiBが上限です。`Extra`は独自のJSONフィールドで、標準フィールドは上書きできません。

`/debug/runtime`の応答はversion、started_unix_ms、captured_unix_ms、metricsを持ちます。collectorは時刻・項目数・有限値・応答サイズを検証して、`sample,timestamp,elapsed_ms,metric,value`のTSVを出します。起動時刻は`go_process_started_unix_ms`としても記録し、分析は同一プロセス内のcounter差分だけを使います。

`task build-measurement`で対象ホスト向けにビルド、`task test-measurement`で検証します。詳細は[Go profile](../../docs/measurement/go-profiling.md)と[任意JSON計測](docs/json-metrics-collector.md)を参照してください。

## 要求bodyとログ出力の計測

`app.ObserveRequestBody(r)`は既存のbodyをラップして実読取量を数えます。handler前に呼び、応答ログへ`BytesRead()`を記録します。ログは共通の`app.NewAccessLogger`を使い、型が決まったentryをqueue経由でまとめて書き出します。snapshotで返す`Metrics()`を既存の指標へマージしてください。

```go
logger, err := app.NewAccessLogger(logFile, app.AccessLoggerOptions{})
if err != nil { log.Fatal(err) }
options := app.Options{
    Snapshot: func() app.Snapshot {
        return app.Snapshot{Metrics: logger.Metrics()}
    },
}
// Pass options to app.Start. After HTTP handlers drain, check logger.Close().
```

`logger.Log(app.AccessLogEntry{...})`のエラーを確認してください。独自loggerが必要な場合は`app.AccessLogStats`で同じcounterを記録できます。記録位置・flush・Closeの契約と集計の意味は[Goアクセスログ](../../docs/measurement/go-access-log.md)を参照してください。新しいcollectorは不要です。

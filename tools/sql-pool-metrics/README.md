# SQL接続プールの計測

`tools/sql-pool-metrics`はアプリのlocalhost debug endpointから接続プール状態を1秒間隔で収集する汎用collectorで、標準計測として有効。
アプリ側に`database/sql.DBStats`相当の情報を返す`GET /debug/sql-pools`を用意する。既定のURLは
`http://127.0.0.1:6060/debug/sql-pools`で、Taskfileの`sql_pool_metrics_url`から変更できる。

## endpointの応答

応答は次の形のJSONにする。

```json
{
  "version": 1,
  "started_unix_ms": 1720000000000,
  "captured_unix_ms": 1720000001000,
  "pools": [
    {
      "database_host": "127.0.0.1",
      "name": "user",
      "role": "shard",
      "shard": 0,
      "max_open_connections": 16,
      "open_connections": 4,
      "in_use": 2,
      "idle": 2,
      "wait_count": 0,
      "wait_duration_ns": 0,
      "max_idle_closed": 0,
      "max_idle_time_closed": 0,
      "max_lifetime_closed": 0
    }
  ]
}
```

時刻はUnix millisecondで、`captured_unix_ms`が`started_unix_ms`より前にならないようにする。
collectorはこの2時刻の差を`elapsed_ms`として保存する。
poolごとに固定の`database_host`、`name`、`role`、`shard`を返し、リクエストやユーザーを識別する値は含めない。
ラベルの組み合わせはsnapshot内で一意にし、接続数・待ち数・累積待ち時間などは非負の値にする。
応答は1 MiB以下、pool数は128以下に収める。

collectorはsnapshotをlong-form TSVにし、`in_use`、`wait_count`、`wait_duration`などの現在値と差分レートを出力する。
`in_use`を`max_open_connections`と比べ、waitの増加をMySQL側の`Threads_running`やCPUと同じRUN・時間窓で見ると、
アプリ接続プールの待ちとDBサーバーの飽和を切り分けやすい。

# Go profileを通常のRUNへ自動収集する

標準計測ではCPUを連続採取し、heap・goroutineのprofileを2秒ごとに保存する。
Go runtimeのgoroutine数、heap量、allocation・GCのcounterは既存のSQL pool collectorで1秒ごとに記録する。
固定秒数後の一回だけのsnapshotは使わず、採取後に負荷期間を選ぶ。
fgprofは任意で既定無効。必要な問いがある場合に追加する。

テンプレートはアプリ実装を持たないため、setupで[共通計測module](../../tools/measurement/README.md)を接続する。
公式ルールを確認し、ローカルのGoアプリへ組み込み、正規deploy経路で反映する。

## 開始・停止の契約

全APP_HOSTSのloopbackに次を公開する。collectorはSSH経由でアクセスする。

| endpoint | 用途 |
|---|---|
| `/debug/pprof/profile?seconds=N&run_id=ID` | CPUを連続採取 |
| `/debug/profiles/snapshots?seconds=N&run_id=ID` | heap・goroutineを2秒ごとにtarへ保存 |
| `/debug/fgprof?seconds=N&run_id=ID` | 任意のwall-clock profile |
| `/debug/pprof/heap`、`allocs`、`goroutine` | 個別調査用の一回のsnapshot |
| `/debug/profiles/status` | `{"cpu":"ID","fgprof":"","snapshots":"ID"}` のようにRUN所有者を返す |
| `POST /debug/profiles/stop?kind=cpu\|fgprof\|snapshots&run_id=ID` | 指定RUNの採取を終了 |

同じ種類の並行収集は拒否する。別RUNへの停止要求は409、停止済みへの再通知は204。
停止後の最終snapshotとindexは、採取中のHTTP応答へ書き出す。
標準`net/http/pprof`だけでは開始確認・停止通知・連続snapshotのendpointは揃わない。

`task bench`は宣言上有効なprofileを開始し、全ホストの開始確認、CPU writerのための1秒の先行収録と再確認、
ベンチ実行、終了マーカー、停止通知、回収、finalizeを行う。開始確認に失敗したら負荷を開始しない。
終了時刻はENTRY_HOSTから取得する。競技サーバー同士の時計差も確認する。

| Taskfile設定 | 意味 |
|---|---|
| `PROFILES_ENABLED` | 既定true。比較等で明示的に停止するときだけfalse |
| `PROFILE_SECONDS` | 既定3600秒の安全上限。通常はベンチ終了通知で停止し、負荷開始秒数の調整は不要 |
| `PROFILE_DELAY` | 既定0s。開始を遅らせない |
| `PROFILE_TIMEOUT_SECONDS` | 上限+15秒。アプリのWriteTimeoutもこれを覆う |
| `PROFILE_READY_TIMEOUT_SECONDS` | 採取開始確認のtimeout |
| `PPROF_BASE_URL` / `SNAPSHOT_PROFILE_URL` / `FGPROF_URL` | 各採取先 |
| `PROFILE_STATUS_URL` / `PROFILE_STOP_URL` | 開始確認・停止通知先 |

## 分析に使える情報と限界

- runtimeの1秒系列で、goroutine増加、heap増加、allocation量、GCの増減を見る。counterは同一プロセス内の差分・経過秒からrateを求める。`go_heap_live_bytes`は直前のGC時点の値。
- 2秒ごとのheapで保持メモリを、goroutineで繰り返し現れるstackとその増減を見る。瞬間的な待ちや待ち時間そのものは測れない。必要ならblock/mutex/trace等を問いに合わせて追加する。
- heap profileには累積allocationも含まれるため、allocsを重複採取しない。分析は負荷期間内の隣接heap profileの差分を使う。ただしGCにより反映が遅れ、サンプリング誤差もある。総allocation量はruntime counterを使い、stack差分は発生箇所の推定に使う。負の差分は無効として残す。
- heapは観測時点の保持状態であり、負荷開始前に確保したオブジェクトも含む。
- CPU・fgprofの通常の出力は初期化等を含む採取全体の集計。各sampleの実時刻がなく、保存後に負荷期間だけへ切り出せない。元のpprofをRUNへ残し、負荷期間のみのDuckDB集計へは混ぜない。CPUの分割採取は標準では追加しない。

`{host}-go-snapshots.tar`には時刻付きpprofと`index.json`を保存する。開始直後・停止時にも採取する。
indexの実際の採取間隔、各export所要時間、artifactの最大欠測間隔を確認する。
遅いexportによる取りこぼしを、設定上の2秒間隔だけから正常と判断しない。
1 profile 8MiB、1 recording 64MiB、4000 profileを上限とし、超過・途中切断・間隔が設定の3倍を超える欠測は検査を失敗させる。

DuckDBには負荷期間内の時点値・関数別集計と、期間内に両端があるallocation差分を取り込む。
全stackを毎回展開せず、詳細調査はtarを任意の一時ディレクトリへ展開して`go tool pprof`で読む。
[テーブルとquery例](../../tools/README.md#go-profileのduckdb閲覧)を参照する。

## アプリへの組み込み

アプリのgo.modから`../../tools/measurement`へ`replace`し、共通moduleをimportする。実装をコピーしない。

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

`DB`は`Stats() sql.DBStats`を満たす値。`Pools`を省略してもruntime指標は出力する。
採取上限`MaxProfileSeconds`は既定3600秒（設定範囲1–3600秒）で、Taskfileの上限と合わせる。
個別HTTP要求でsecondsを省略した場合は最大30秒。
既定のlistenerは`127.0.0.1:6060`、WriteTimeoutは上限+15秒。起動失敗は同期的に返す。
`Address`を変える場合はcollector側も揃える。独自listenerでは`app.NewHandler`をプロセス内に一つだけ作る。
アプリ固有指標は`Snapshot` callbackで渡す。

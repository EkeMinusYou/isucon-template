# Go profileを通常のRUNへ自動収集する

CPU・heap・allocs・goroutineのpprofは標準計測として既定で有効。テンプレートはアプリ実装を持たないため、setupでendpointを導入する。
fgprofは任意の追加計測で既定無効。setupの完了条件には含めず、必要な場合に`task fgprof-collect`で明示的に採取する。
[共通計測module](../../tools/measurement/README.md)をアプリから読み込み、起動時に明示的に開始する。
競技ルールは `docs/official/` を確認し、ローカルのGoアプリに追加して正規deploy経路で反映する。
アプリ固有のハンドラー・DB・nginx構成は含まない。

## 有効化する条件

全APP_HOSTSで標準pprofと開始確認・停止通知のendpointをloopbackに公開する。fgprofのendpointは利用する場合に確認する。テンプレートではSSH経由でアクセスする。

| endpoint | 用途 |
|---|---|
| `/debug/pprof/profile?seconds=N&run_id=ID` | CPU profile |
| `/debug/fgprof?seconds=N&run_id=ID` | 任意のgoroutine wall-clock profile |
| `/debug/pprof/heap`、`allocs`、`goroutine` | snapshot |
| `/debug/profiles/status` | 実行中の種類とRUN IDをJSONで返す |
| `POST /debug/profiles/stop?kind=cpu\|fgprof&run_id=ID` | 指定RUNの採取へ停止を通知する |

開始確認は `{"cpu":"RUN_ID","fgprof":""}` のような空白なしのJSONを前提とし、有効な採取対象だけを確認する。
未実行の種類は空文字を返す。標準`net/http/pprof`だけではこのendpointは存在しない。
独自adapterを使う場合は`collectors.yaml`の`ready`もその形式に合わせる。
同じ種類の並行収集は拒否し、CPUとfgprofの併用は許容する。
停止通知はRUN IDを照合し、別RUNの採取には409を返す。停止済みの採取への再通知は204を返す。
通知は採取ハンドラーへ伝え、profileの書き出しは採取中のHTTP応答で完了させる。

## 時間と開始順序

Taskfileの次の値を競技に合わせる。120秒・40秒は例であり、競技固有の保証値ではない。

| 設定 | 意味 |
|---|---|
| `PROFILES_ENABLED` | 既定true。比較等で採取を停止する場合だけfalseに変更 |
| `PROFILE_SECONDS` | 初期化・整合性チェック・負荷本体・終了処理と余裕を覆う採取上限。通常はベンチ終了通知で早期停止 |
| `PROFILE_DELAY` | 自動収集では通常0s。遅延を増やすなら開始確認timeoutも調整 |
| `SNAPSHOT_PROFILE_DELAY` | 初期化後かつ負荷終了前にsnapshotを取得する遅延 |
| `PROFILE_TIMEOUT_SECONDS` | 既定では取得秒数+15秒。アプリのWriteTimeoutも十分長くする |
| `PROFILE_READY_TIMEOUT_SECONDS` | SSH到達後に採取開始を待つ秒数 |
| `PPROF_BASE_URL` / `FGPROF_URL` / `PROFILE_STATUS_URL` | adapterの公開先。fgprofのsecondsはPROFILE_SECONDSに追従 |
| `PROFILE_STOP_URL` | RUN IDを照合して採取へ停止を通知するendpoint |

通常の`task bench`が`group: profiles`の有効な宣言を選び、収集開始、全ホストの開始確認、
1秒の先行収録、開始状態の再確認、負荷開始、終了時刻の記録、停止通知、profile回収待ち、finalizeを順に行う。
停止通知は全APP_HOSTSの有効なCPU・fgprof宣言へ並列に送る。失敗した場合は採取上限まで待って回収・finalizeし、エラーを返す。
ポータルの待機が長い場合など、採取窓が負荷を覆わなければartifact検査は失敗する。
開始確認失敗時は負荷を開始せず、失敗RUNとして回収を試みる。
開始通知はprofiler起動成功後に公開する。GoのCPU profile writerは非同期に起動するため、
通知後も1秒の先行収録時間を取る。この余裕は任意の起動遅延や大きなホスト時計差を保証せず、
収録時間にはこの1秒も含めて余裕を持たせる。artifactの時間窓検査は緩和しない。

新しいRUNでは宣言上有効なprofile×APP_HOSTS、各NGINX_HOSTSの圧縮access log、user-transitionを必須にする。
profileの形式・sample種別・時刻と、ログのzstd・JSON・必須列を検査する。
ログは空でも保存し、trafficのないホストと回収失敗を区別する。
データは `go tool pprof` と[DuckDBのpprof表](../../tools/README.md#go-profileのduckdb閲覧)で読む。

## アプリへの組み込み

profileの実装をアプリへコピーせず、`github.com/EkeMinusYou/isucon-template/tools/measurement/app`をimportする。
このリポジトリではアプリのgo.modから`../../tools/measurement`へ`replace`して使う。
新しい競技のアプリでも相対パスを合わせて共通moduleへ接続する。

```go
import "github.com/EkeMinusYou/isucon-template/tools/measurement/app"

server, err := app.Start(app.Options{
    MaxProfileSeconds: 180,
    Pools: []app.Pool{{
        DB: db, DatabaseHost: "db.local", Name: "main", Role: "primary",
    }},
})
if err != nil {
    log.Fatal(err)
}
defer server.Close()
```

`DB`は`Stats() sql.DBStats`を満たす値（`*sql.DB`や`*sqlx.DB`）。profileだけが必要なら`Pools`は省略できる。
起動上限は1–3600秒、既定180秒。Taskfileの`PROFILE_SECONDS`以上に設定する。
既定のlistenerは`127.0.0.1:6060`で、WriteTimeoutは上限+15秒。
`Address`で変更する場合はTaskfileの取得URLも揃え、公開trafficから到達させない。
起動失敗は呼び出し元へ同期的に返す。エラーを無視したまま自動収集を有効にしない。

独自listenerがある場合は`app.NewHandler`を使う。プロセス内でhandlerを一つだけ作成する。
アプリ固有の内部指標は`Snapshot` callbackで渡せる。詳細は[共通moduleのREADME](../../tools/measurement/README.md)を参照。

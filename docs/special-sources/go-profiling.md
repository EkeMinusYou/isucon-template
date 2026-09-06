# Go profileを通常のRUNへ自動収集する

profileは標準計測として既定で有効。テンプレートはアプリ実装を持たないため、setupでendpointを導入する。
以下は標準pprof・fgprof・開始確認endpointをアプリに組み込む例であり、自動適用されない。
競技ルールは `docs/official/` を確認し、ローカルのGoアプリに追加して正規deploy経路で反映する。
アプリ固有のハンドラー・DB・nginx構成は含まない。

## 有効化する条件

全APP_HOSTSで次のendpointをloopbackに公開する。テンプレートではSSH経由でアクセスする。

| endpoint | 用途 |
|---|---|
| `/debug/pprof/profile?seconds=N&run_id=ID` | CPU profile |
| `/debug/fgprof?seconds=N&run_id=ID` | goroutineのwall-clock profile |
| `/debug/pprof/heap`、`allocs`、`goroutine` | snapshot |
| `/debug/profiles/status` | 実行中の種類とRUN IDをJSONで返す |

開始確認は `{"cpu":"RUN_ID","fgprof":"RUN_ID"}` のような空白なしのJSONを前提とする。
未実行の種類は空文字を返す。標準`net/http/pprof`だけではこのendpointは存在しない。
独自adapterを使う場合は`collectors.yaml`の`ready`もその形式に合わせる。
同じ種類の並行収集は拒否し、CPUとfgprofの併用は許容する。

## 時間と開始順序

Taskfileの次の値を競技に合わせる。120秒・40秒は例であり、競技固有の保証値ではない。

| 設定 | 意味 |
|---|---|
| `PROFILES_ENABLED` | 既定true。比較等で採取を停止する場合だけfalseに変更 |
| `PROFILE_SECONDS` | 初期化・整合性チェック・負荷本体・終了処理と余裕を覆う秒数 |
| `PROFILE_DELAY` | 自動収集では通常0s。遅延を増やすなら開始確認timeoutも調整 |
| `SNAPSHOT_PROFILE_DELAY` | 初期化後かつ負荷終了前にsnapshotを取得する遅延 |
| `PROFILE_TIMEOUT_SECONDS` | 既定では取得秒数+15秒。アプリのWriteTimeoutも十分長くする |
| `PROFILE_READY_TIMEOUT_SECONDS` | SSH到達後に採取開始を待つ秒数 |
| `PPROF_BASE_URL` / `FGPROF_URL` / `PROFILE_STATUS_URL` | adapterの公開先。fgprofのsecondsはPROFILE_SECONDSに追従 |

通常の`task bench` / `task bench-manual`が`group: profiles`の有効な宣言を選び、収集開始、全ホストの開始確認、負荷開始、
profile回収待ち、finalizeを順に行う。手動モードでは案内後すぐに負荷を開始する。
ポータルの待機が長い場合など、採取窓が負荷を覆わなければartifact検査は失敗する。
開始確認失敗時は負荷を開始せず、失敗RUNとして回収を試みる。

分割操作する場合は開始確認と回収待ちを呼び出し側で管理する。

```sh
task before-bench PROFILES_ENABLED=true
task profiles-collect &
profile_pid=$!
task profiles-ready
# Start the benchmark immediately in another terminal and wait for completion.
wait "$profile_pid"
task after-bench SCORE=12345
```

新しいRUNでは宣言上有効なprofile×APP_HOSTS、各NGINX_HOSTSの圧縮access log、user-transitionを必須にする。
profileの形式・sample種別・時刻と、ログのzstd・JSON・必須列を検査する。
ログは空でも保存し、trafficのないホストと回収失敗を区別する。
データは `go tool pprof` と[DuckDBのpprof表](../../tools/README.md#go-profileのduckdb閲覧)で読む。

## アプリへの組み込み例

`github.com/felixge/fgprof`をアプリのgo.modへ追加する。以下の例はv0.9.5で検証した。
`package main`はアプリに合わせる。起動時に、競技の採取時間以上の上限を渡して
`startProfileServer(maxSeconds)`を呼び、戻り値のエラーをアプリの起動方針に従って扱う。
例はloopback:6060、設定可能な上限1–3600秒とし、WriteTimeoutを上限+15秒にする。
公開traffic用muxへ登録しない。起動時エラーを無視したまま自動収集を有効にしない。

```go
package main

import (
	"encoding/json"
	"fmt"
	"log"
	"net"
	"net/http"
	"net/http/pprof"
	"strconv"
	"sync"
	"time"

	"github.com/felixge/fgprof"
)

func profileHandler(maxSeconds int) http.Handler {
	mux := http.NewServeMux()
	var stateMu sync.Mutex
	active := map[string]string{"cpu": "", "fgprof": ""}
	mux.HandleFunc("/debug/profiles/status", func(w http.ResponseWriter, r *http.Request) {
		stateMu.Lock()
		defer stateMu.Unlock()
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(active)
	})
	// CPU and goroutine wall-clock sampling can run together. Reject duplicate
	// captures of the same kind and expose RUN ownership to the collector gate.
	bounded := func(kind string, next http.Handler) http.Handler {
		var sampling sync.Mutex
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			seconds := maxSeconds
			if value := r.URL.Query().Get("seconds"); value != "" {
				var err error
				seconds, err = strconv.Atoi(value)
				if err != nil || seconds < 1 || seconds > maxSeconds {
					http.Error(w, fmt.Sprintf("seconds must be between 1 and %d", maxSeconds), http.StatusBadRequest)
					return
				}
			}
			if !sampling.TryLock() {
				http.Error(w, "a sampling profile is already running", http.StatusConflict)
				return
			}
			defer sampling.Unlock()
			stateMu.Lock()
			active[kind] = r.URL.Query().Get("run_id")
			if active[kind] == "" {
				active[kind] = "manual"
			}
			stateMu.Unlock()
			defer func() {
				stateMu.Lock()
				active[kind] = ""
				stateMu.Unlock()
			}()
			query := r.URL.Query()
			query.Set("seconds", strconv.Itoa(seconds))
			r.URL.RawQuery = query.Encode()
			next.ServeHTTP(w, r)
		})
	}
	mux.Handle("/debug/pprof/profile", bounded("cpu", http.HandlerFunc(pprof.Profile)))
	mux.Handle("/debug/fgprof", bounded("fgprof", fgprof.Handler()))
	for _, name := range []string{"heap", "allocs", "goroutine"} {
		mux.Handle("/debug/pprof/"+name, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			// These endpoints are snapshots; delta profiles need a separate policy.
			if r.URL.Query().Has("seconds") {
				http.Error(w, "snapshot profiles do not accept seconds", http.StatusBadRequest)
				return
			}
			pprof.Handler(r.URL.Path[len("/debug/pprof/"):]).ServeHTTP(w, r)
		}))
	}
	return mux
}

func startProfileServer(maxSeconds int) error {
	if maxSeconds < 1 || maxSeconds > 3600 {
		return fmt.Errorf("profile duration limit must be between 1 and 3600 seconds")
	}
	listener, err := net.Listen("tcp", "127.0.0.1:6060")
	if err != nil {
		return fmt.Errorf("profile listener unavailable: %w", err)
	}
	server := &http.Server{
		Handler:           profileHandler(maxSeconds),
		ReadHeaderTimeout: 5 * time.Second,
		WriteTimeout:      time.Duration(maxSeconds+15) * time.Second,
		IdleTimeout:       30 * time.Second,
	}
	go func() {
		if err := server.Serve(listener); err != nil && err != http.ErrServerClosed {
			log.Printf("profile server stopped: %v", err)
		}
	}()
	return nil
}
```

## 検証と停止

短い採取時間で全APP_HOSTSへの到達、開始確認、5種類の解析を確認してから、
競技に合わせた時間へ戻してユーザーのベンチRUNで検証する。
`task artifacts-run RUN=runs/<RUN_ID>`で存在・内容・時間窓を確認する。
profile有効化による負荷は同じアプリ・配置・負荷条件で比較し、スコアだけから判断しない。

次RUNのprofileを止めるには`PROFILES_ENABLED=false`へ戻す。
no-collectorsタスクは周期collectorとprofileを無効化するが、ログは出力・回収を続ける。
進行中の採取は設定した上限内で終了するため、通常の回収を待つ。
アプリからadapterも取り除く場合は、進行中RUNがないことを確認し、ローカルで呼び出しと実装を戻して
build・deployのdry-run・正規deployを行う。保存済みRUNは変更しない。

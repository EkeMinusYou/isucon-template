# Portal benchmark tool

Goの汎用ツール。`start`でジョブを投入し、`poll`で完了を待ち、ログとスコア・合否を取得する。
URL、認証、リクエスト、応答の解釈、完了条件はJSON設定に置く。競技固有のコードや生成protobufコードへの依存はない。

各段階でHTTPまたは外部コマンドを選べる。`poll`と結果取得が別endpointなら、任意の`result`段階を追加する。
ログは取得済みの最新snapshotを保存し、完了時に標準出力へ出す。途中で通信失敗やタイムアウトになった場合も取得済みログを出す。
ポータルが公開していないログは取得できない。

## このリポジトリで使う

既定の設定は[競技ごとのポータル設定](../contest/portal-bench.json)。初期値はISUCON12形式の設定例で、
競技開始時にWebUIのHARを根拠に要求・応答・完了条件を合わせる。
`base_url`と`vars.target_id`を実際のポータル・対象インスタンスへ合わせる。
対象IDとホストの対応はポータルのサーバーリストで確認する。

ポータルへログイン済みのブラウザからCookieヘッダー全体とCSRFトークンを取得し、環境変数に入れる。
設定例に認証情報は含めていない。ルートの`.env.local`へ書くとTaskが自動で読み込む。
`.env.local`とルートのHARはGit管理対象外。環境変数をexportしている場合はそちらが優先される。

```sh
export PORTAL_COOKIE='Cookieヘッダー全体'
export PORTAL_CSRF_TOKEN='ログイン済みページのCSRFトークン'
task portal-bench-check
```

`portal-bench-check`は設定と必要な環境変数を検証するだけで、通信も外部コマンド実行もしない。
設定が揃ったら、次のコマンドでベンチを実行します:

```sh
task bench
# collectorなしで実行する場合
task bench MEASURECTL_COLLECT_FLAGS=-no-collectors PROFILES_ENABLED=false
```

既存の`before-bench`による配布物・service検査、operation lock、collector開始、`after-bench`によるfinalizeを使う。
`runs/<RUN_ID>/bench.log`へ取得ログと結果marker、`benchmark-result.txt`へ抽出した原文を保存する。
失敗時もRUNをfinalizeする。原文ファイルはmanifestの任意成果物として一覧に出る。
スコア・合否・エラーの競技固有の解釈は、`tools/contest/bench-patterns.json`と`analysis-schema/`で設定する。ベンチマーカーの進捗ログは、取得できた場合に分析に利用する。

他の設定へ切り替える例:

```sh
export PORTAL_URL='https://実際のポータル'
export PORTAL_TARGET_ID='対象インスタンスID' # ISUCON12用
task portal-bench-check PORTAL_BENCH_CONFIG=tools/contest/portal/isucon12.json
task bench PORTAL_BENCH_CONFIG=tools/contest/portal/isucon12.json
task bench PORTAL_BENCH_CONFIG=tools/contest/portal/isucon13.json
```

継続して使う設定は、Taskfileのcontest values区画の`CONTEST_PORTAL_BENCH_CONFIG`へ指定する。

## ビルドと結果の再取得

`task build-portal-bench`でローカル環境向けにビルドできる。Go標準ライブラリのみを使う。
競技サーバーへ配布するツールではない。

```sh
tools/portal-bench/portal-bench -config path/to/portal.json -check
# 通信が途切れたジョブの結果だけ再取得。新規投入はしない。
tools/portal-bench/portal-bench -config path/to/portal.json -job-id 42 -output-dir path/to/results
```

単独実行で出力先を省略した場合は標準出力のみ。`-base-url`でURLを上書きできる。
このリポジトリで新しいジョブを投入するときは、ロックと配布物検査のある`task bench`を入口にする。
`-job-id`の再取得は新しい負荷区間を作らないので、`bench`で新規RUNにしない。

終了コードは正常完了`0`、明示的なベンチ失敗`1`、設定・通信・タイムアウト等のエラー`2`。
判定設定を省略した場合は合否markerを出さない。判定値が設定した成功・失敗値のどちらにも一致しなければ、成功を推測しない。

## 設定する項目

| 項目 | 内容 |
| --- | --- |
| `base_url` | HTTPリクエストの基準URL |
| `vars` | 各段階へ渡す文字列変数。固定値または`env`を使う |
| `headers` | 全HTTP段階に共通する認証などのヘッダー |
| `timeout` | 投入から取得完了までの上限。既定`20m` |
| `request_timeout` | HTTP／コマンド1回の上限。既定`30s` |
| `poll_interval` | 未完了の応答、再試行後に待つ時間。既定`5s` |
| `poll_retries` | 連続した一時的なpoll通信失敗の再試行回数。既定`0` |
| `start` | 投入方法。応答から`job_id`を必ず抽出 |
| `poll` | 状態取得方法。`outcome.status`のフィールドを抽出 |
| `result` | 任意。完了後に結果・ログを別途取得する方法 |
| `outcome` | 完了・失敗条件、合否・スコア・ログに使うフィールド名 |

各段階の`http`は`method`、`url`、任意の`headers`、`body`または`body_base64`を指定する。
相対URLは`base_url`で解決する。段階ごとのヘッダーが共通ヘッダーを上書きする。
`body_base64`はBase64をデコードして送信するため、protobuf等のバイナリ要求も表現できる。

`http`の代わりに`command`へ引数配列を指定すると、標準出力を応答として読み取る。
`env`へそのコマンドに渡す環境変数を指定できる。標準エラーは診断ログとして渡す。
シェル展開は行わない。既存CLIによる認証・WebSocket等、HTTP段階で表せない仕様も、応答をJSON等に変換するコマンドを設定して接続できる。
そのCLIやadapterは利用者が用意し、認証情報を標準エラーへ出さないようにする。

```json
{
  "command": ["portal-cli", "result", "--job", "{{.job_id}}", "--json"],
  "env": {"PORTAL_API_TOKEN": "{{env \"PORTAL_API_TOKEN\"}}"},
  "response": {
    "format": "json",
    "fields": {
      "state": {"path": "/status"},
      "log": {"path": "/output", "default": ""}
    }
  }
}
```

### 応答の抽出

`response.format`と`response.fields`で、必要なフィールドだけを取り出す。

| format | `path`の書き方 |
| --- | --- |
| `json` | JSON Pointer。`/job/id`、配列なら`/jobs/0/id`。整数IDの精度を保持 |
| `protobuf` | フィールド番号をドット区切り。`1.18.5.2`。繰り返しフィールドの添字は`1[0].1` |
| `regex` | capture groupが1個のGo正規表現。複数行は`(?s)`を使う |
| `text` | 本文全体。`path`は空文字 |

抽出値は文字列として扱う。`default`は欠落・JSON nullのときだけ使う文字列。
省略すると欠落はエラーになる。protobufではproto3のゼロ値が送信されないため、状態`0`やboolの`false`などを明示しておく。
不正なバイナリやJSONはdefaultで隠さずエラーになる。

protobufの`type`は`uint`、`int`、`sint`、`bool`、`string`、`bytes`、`float32`、`float64`。
`bytes`はBase64文字列を返す。省略時はwire typeに応じて`uint`または`string`として読む。
ネストしたmessageと繰り返しフィールドを扱い、packed配列やprotobuf groupは対象外。必要なら外部コマンドで変換する。

`regex`では`type: "html_text"`を指定すると`<br>`を改行にしてタグを除去し、HTML entityを戻す。
これは設定したHTML断片のテキスト抽出用で、JavaScript実行やDOM解析は行わない。
`replace: [{"pattern": "[,\\s]", "value": ""}]`で桁区切りなどを除去でき、`trim: true`で前後の空白を取り除ける。

`outcome.status`で指定した値が`done_values`に入ると完了、`failed_values`に入ると異常終了。
`outcome.pass`が`pass_values`／`fail_values`に一致したときに`BENCHMARK_PASS`／`BENCHMARK_FAIL`を出す。
`outcome.score`を指定すると有限な数値を検査して`SCORE: <値>`を出す。
`outcome.logs`は順に連結するログフィールド名の配列。各pollのログは累積snapshotとして扱う。
差分しか返さないAPIは、外部コマンド側で累積ログにして返す。

### テンプレート

URL・ヘッダー・本文・コマンド引数・env値にはGoのtext/templateを使える。
`vars`の値は固定値または環境変数から解決し、互いを参照しない。
`base_url`と`job_id`は予約名。各応答の抽出値は次の段階へ引き継ぐ。

| 記法 | 用途 |
| --- | --- |
| `{{.job_id}}` | 投入応答から取得したジョブID |
| `{{.base_url}}` | 基準URL |
| `{{.target_id}}` | `vars`や前段の応答で指定した変数 |
| `{{env "PORTAL_COOKIE"}}` | 必須の環境変数。未設定・空文字は開始前にエラー |
| `{{urlquery .job_id}}` | URLへ埋め込む値をエスケープ |
| `{{json .target_id}}` | JSON文字列としてエスケープ。外側の引用符は不要 |
| `{{base64 .payload}}` | 文字列をBase64へ変換 |
| `{{protoUint 1 .target_id}}` | varintフィールド1個をBase64にする。対象IDのprotobuf要求向け |

投入要求は自動再試行しない。応答を失った場合はポータルでジョブの有無を確認し、存在するなら`-job-id`で回収する。
pollでは通信失敗・429・5xxだけを設定回数まで再試行する。認証切れのredirect、4xx、解釈不能な応答はエラー。
HTTP redirectには追従しない。HTTP応答・コマンド標準出力の上限はそれぞれ64MiB。

## ISUCON12・13との確認

| 設定 | ローカルのポータル実装で確認した契約 | 保存するログ |
| --- | --- | --- |
| [isucon12.json](../contest/portal/isucon12.json) | POST `/api/contestant/benchmark_jobs`、GET `.../<id>`、protobuf状態0〜4・result.passed | Execution.stdoutとstderr |
| [isucon13.json](../contest/portal/isucon13.json) | POST `/contest/jobs/enqueue/`のJSON ID、GET `/contest/jobs/<id>/`のHTML、AJAX／CSRFヘッダー | 参加者向けMessage |

ISUCON12では`proto/isuxportal/services/contestant/benchmark.proto`、`resources/benchmark_job.proto`、
`resources/benchmark_result.proto`、contestant controllerを確認した。
ISUCON13では`isucon/portal/contest/urls.py`、`views.py`、`models.py`、`templates/includes/job_detail_part.html`を確認した。
ISUCON13の参加者向けページにはstdout／stderrが表示されず、スタッフ向けの条件分岐にある。
参加者の権限で取得できるMessageだけを設定に含めている。

検証はこれらの契約に沿うローカルHTTPサーバーで行う。競技のHARを再生するテストを追加する場合も、
HARやその認証情報をリポジトリへ追加しない。

```sh
cd tools/portal-bench
go test ./...
```

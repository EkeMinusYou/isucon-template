# Goのアクセスログ

既定のnginx＋Go構成では[nginxアクセスログ](nginx-access-log.md)を標準集計に使います。この文書はGo内部の追加診断、または明示的にGoを入口に変更した場合の契約です。二つのログを同じ入口の要求として重複集計しません。

HTTP入口を担当するGoプロセスから、1要求につき1行のJSONを`ACCESS_LOG_PATH`へ出します。標準集計と負荷期間推定はこのログを読みます。

| 列 | 内容 |
|---|---|
| `msec` | 応答完了時刻。Unix秒の小数で、ミリ秒整数ではない |
| `method`, `uri`, `status` | HTTP method、要求URI、応答status |
| `response_time` | 要求受付から応答書き出し完了までの秒数 |
| `body_bytes` | 応答bodyの送信byte数 |
| `request_body_bytes` | アプリが実際に読み取った要求bodyのbyte数 |
| `request_content_length` | 要求のContent-Length。不明な場合は負値または省略 |
| `session_id` | 操作を結び付ける安定した識別値。生の認証tokenは出さない |
| `connection`, `connection_requests` | ホスト・起動世代内で一意な接続IDと接続内の要求数。標準で記録し、取得できなければ省略して理由を示す |
| `ssl_session_reused` | TLS再利用時`r`、新規時`.`。未観測なら省略 |

JSON encoderでescapeします。loggerは並行書込みを直列化し、`O_APPEND`で開いて1秒以内にflushします。RUN開始前に同じファイルをtruncateするため、書込み位置への依存や長時間の未flushを避けます。ディレクトリ・ファイルはSSH操作ユーザーがtruncate・回収できる権限へ設定します。書込み失敗・欠落を検出し、成功扱いで隠しません。

Goアクセスログを追加する場合は共通moduleの`app.NewAccessLogger`と`app.AccessLogEntry`を使う。既定は4,096件のqueue、5msごとのまとめ書き、100msごとのflushで、型の決まったJSONを応答経路の外で生成する。queueが満杯なら空きを待ち、記録を黙って捨てない。サイズ・間隔は`AccessLoggerOptions`で変更できる。

`Log`へ渡したentryとその参照先は変更しない。追加の診断項目は`Details`へ型を定義して渡せるが、標準項目をそこへ移さない。HTTP受付を止め、handlerが終了してから`Close`を呼ぶと残りを排出・flushする。ファイル自体は呼出側で閉じる。`Log`・`Close`のエラーを処理し、非同期書出しの失敗は`Err()`と`Metrics()`でも確認する。

middlewareでResponseWriterを包む場合は、暗黙の200、途中切断、Flush・Hijack等の必要なinterfaceを維持します。SSE等ではハンドラー終了と接続終了の意味を合わせ、単にhandler開始時刻を完了時刻として記録しません。

bootstrapで全HTTP入口のログと`app_traffic` roleを揃えます。`alp.yml`、`user-transition-routes.json`、`analysis/schema/access-log.sql`のAPI分類・動的ID正規化を揃え、ログ出力からalp・操作遷移・負荷期間のDuckDB集計まで確認してください。

接続間隔の計算は逐次HTTP/1.1を前提とします。HTTP/2の並行streamでは接続単位の差を利用者の待ち時間と解釈せず、必要なら別の観測を追加します。取得できない指標を0で補いません。

`connection_reuse`で、負荷期間内の接続数・再利用された接続数・接続あたり要求数を比較する。
再起動でIDを使い回す場合は起動世代を含む番号にするなど、同じホストのログ内で衝突させない。
TCPのRTT・再送数などは、通信待ちの診断で必要になったときに採取範囲と負荷を決めて追加する。

## 受信量を記録する

Goで実読取量を測る場合は`app.ObserveRequestBody(r)`をhandlerの読み取り前に呼び、返り値の`BytesRead()`を応答時に`request_body_bytes`へ記録する。空bodyでは返り値はnilだが、`BytesRead()`は0を返す。要求全体を計測のために読み込まず、handlerが実際に消費した分だけを数える。`request_content_length`には`r.ContentLength`を記録し、実読取量と区別する。

`http_traffic`と`http_traffic_series`には実読取量の`sum_request_body_bytes`・`request_body_samples`、ヘッダー値の`sum_request_content_length`・`request_content_length_samples`がある。API・時間帯ごとに比較でき、旧ログや不明値は0 byteで補わず、取得件数とNULLで示す。

## ログ出力状況を記録する

共通ロガーの`Metrics()`を`Options.Snapshot`の指標へ加える。独自loggerが必要な場合は`app.AccessLogStats`を使い、出力対象に受理したとき`Enqueued()`、1行の書き出しに成功したとき`Written()`、破棄したとき`Dropped()`を呼ぶ。`Metrics(len(queue))`をsnapshotへ加え、同期loggerのqueue長は0とする。

1秒ごとのruntime計測から`access_log_health`で記録数・書出数・破棄数の期間差分、queue長の最大・最終値を確認できる。共通ロガーは書出し失敗も`access_log_errors_total`へ記録し、`output_errors`で確認できる。プロセス再起動をまたぐ差分は計算せず、未計測の値はNULLのまま残す。書出数はwriterへの出力成功数であり、bufferのflush完了やディスク永続化の保証ではない。

負荷後にqueueとbufferを排出し、記録数・書出数・破棄数を照合する。負荷中のqueue残量や記録数と書出数の差だけを、ログ欠損と決めつけない。計測のCPU負荷は既存profileで確認し、これらのcounterをCPU負荷そのものとは解釈しない。

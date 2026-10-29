# Goのアクセスログ

HTTP入口を担当するGoプロセスから、1要求につき1行のJSONを`ACCESS_LOG_PATH`へ出します。標準集計と負荷期間推定はこのログを読みます。

| 列 | 内容 |
|---|---|
| `msec` | 応答完了時刻。Unix秒の小数で、ミリ秒整数ではない |
| `method`, `uri`, `status` | HTTP method、要求URI、応答status |
| `response_time` | 要求受付から応答書き出し完了までの秒数 |
| `body_bytes` | 応答bodyの送信byte数 |
| `session_id` | 操作を結び付ける安定した識別値。生の認証tokenは出さない |
| `connection`, `connection_requests` | ホスト・起動世代内で一意な接続IDと接続内の要求数。標準で記録し、取得できなければ省略して理由を示す |
| `ssl_session_reused` | TLS再利用時`r`、新規時`.`。未観測なら省略 |

JSON encoderでescapeします。loggerは並行書込みを直列化し、`O_APPEND`で開いて1秒以内にflushします。RUN開始前に同じファイルをtruncateするため、書込み位置への依存や長時間の未flushを避けます。ディレクトリ・ファイルはSSH操作ユーザーがtruncate・回収できる権限へ設定します。書込み失敗・欠落を検出し、成功扱いで隠しません。

middlewareでResponseWriterを包む場合は、暗黙の200、途中切断、Flush・Hijack等の必要なinterfaceを維持します。SSE等ではハンドラー終了と接続終了の意味を合わせ、単にhandler開始時刻を完了時刻として記録しません。

bootstrapで全HTTP入口のログと`app_traffic` roleを揃えます。`alp.yml`、`user-transition-routes.json`、`analysis/schema/access-log.sql`のAPI分類・動的ID正規化を揃え、ログ出力からalp・操作遷移・負荷期間のDuckDB集計まで確認してください。

接続間隔の計算は逐次HTTP/1.1を前提とします。HTTP/2の並行streamでは接続単位の差を利用者の待ち時間と解釈せず、必要なら別の観測を追加します。取得できない指標を0で補いません。

`connection_reuse`で、負荷期間内の接続数・再利用された接続数・接続あたり要求数を比較する。
再起動でIDを使い回す場合は起動世代を含む番号にするなど、同じホストのログ内で衝突させない。
TCPのRTT・再送数などは、通信待ちの診断で必要になったときに採取範囲と負荷を決めて追加する。

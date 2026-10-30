# Declared validation

公開HTTP応答と、競技側で実装したテストを実行し、結果を保存する汎用runnerです。
通知方式、業務API、操作間の順序、キャッシュ期限は共通実装で決めません。

```sh
task validate
```

`tools/contest/validation.json`へ検査を宣言します。`task setup-check`は構文だけを検査し、
`task setup-postflight`はrole・接続・配布物の確認とともに実行します。
配置、応答、操作間の条件を変更した場合は、影響する宣言を更新して再実行してください。
通常のベンチごとには再実行しません。

```json
{
  "version": 1,
  "http": [
    {
      "id": "public-response",
      "url": "${PUBLIC_BASE_URL}/example",
      "method": "GET",
      "timeout_seconds": 10,
      "expect": {
        "status": 200,
        "headers": {
          "Content-Type": {"matches": "^text/"},
          "X-Internal": {"absent": true}
        },
        "body": {"contains": "expected text"}
      }
    }
  ],
  "commands": [
    {
      "id": "operation-contract",
      "argv": ["go", "-C", "webapp/go", "test", "-run", "TestPublicContract", "./..."],
      "timeout_seconds": 60
    }
  ]
}
```

この例のURL・期待値・テスト名は、外部仕様に合わせて差し替えます。HTTP検査の期待statusは必須です。
ヘッダーは文字列による完全一致、`equals`、`matches`、`absent: true`を使えます。
bodyは`contains`と、受信した未展開byte列の`sha256`を指定でき、上限は2 MiBです。
リダイレクトは追跡せず、その応答自体を検査します。本文条件のない検査はヘッダー確認後に切断します。
認証等は`headers`の値に`${ENV_NAME}`を指定し、リポジトリ外の環境変数から渡します。
必要なら`request_body`と`ca_file`にも環境変数を使えます。TLS検証は無効化しません。
`timeout_seconds`はHTTPでは接続・読み取りの待ち、commandsではプロセス全体の上限です。

commandsはargvをshellへ展開せずに実行します。`cwd`は省略時にリポジトリrootです。
複数利用者の操作や並行実行の正当性は、外部仕様から競技側のテストに実装してください。
サーバー側の書き込み完了と受信側の処理完了は別です。テストが観測していない順序まで確認済みにはできません。
検査に更新操作を含める場合は、対象と副作用を確認して実行します。task経由では既存の操作ロックを使います。

結果は`generated/validation/latest.json`へ保存します。失敗は結果を保存して終了コード1を返します。
commandsのstdout/stderrはそれぞれ64 KiBまで保存し、timeout時は起動したプロセス群を終了します。
HTTPの応答本文や認証ヘッダー値は結果へ保存せず、宣言で参照した環境変数値はcommand出力でも伏せます。
任意のテスト出力には他の機密情報が含まれる可能性があるため、結果ファイルはGit対象外・0600で保存します。

- 空の宣言は`not_configured`。検証成功として扱いません。
- 対象外の検査は`id`と`skip_reason`で理由を宣言でき、`skipped`として記録します。
- `before-bench`は直近結果をRUNの`contract-validation.json`へ保存します。これは再検証ではありません。
- 元のcommit・tracked差分・宣言のSHA-256を記録します。成功結果のsource・宣言が変わった場合は
  `stale`へ変更して保存します。外部データ、環境変数、時刻などの同一性はこの判定では保証しません。
- 一度の成功は宣言した条件での観測結果です。公式ベンチの正当性判定を代替しません。

RUNの検査結果もGit対象外です。必要な判断は会話で報告し、履歴を保存する場合は認証情報等を除いてください。

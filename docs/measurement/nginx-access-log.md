# nginx access logの計測項目

標準計測はnginxの`/var/log/nginx/access.log`をRUNごとに回収し、`alp`、upstream集計、必要に応じてuser-transition集計へ渡す。
nginxの設定元（通常は`nginx/nginx.conf`またはvhost設定）でJSON access logを出し、実際のファイルパスは
`tools/measurectl/digesters.yaml`の`access` sourceと一致させる。

## JSONに含める列

標準集計では次の値をJSON列として出す。

- 共通: `msec`、`method`、`uri`、`status`、`response_time`、`body_bytes`
- upstream分析: `upstream_time`、`upstream_addr`、`upstream_status`、`cache_status`
- user-transitionを使う場合: Cookie由来のセッション識別子を入れる専用列。既定名は`session_id`

共通列とupstream列は、たとえば次のように設定できる。既存の`http {}`へ追加し、access logの出力先を標準pathに合わせる。

```nginx
log_format isucon_json escape=json
  '{"msec":"$msec",'
  '"method":"$request_method",'
  '"uri":"$uri",'
  '"status":$status,'
  '"response_time":$request_time,'
  '"body_bytes":$body_bytes_sent,'
  '"upstream_time":"$upstream_response_time",'
  '"upstream_addr":"$upstream_addr",'
  '"upstream_status":"$upstream_status",'
  '"cache_status":"$upstream_cache_status",'
  '"session_id":"$upstream_http_x_measurement_session_id"}';
access_log /var/log/nginx/access.log isucon_json;
```

`uri`にはクエリ文字列や個人情報を含めず、正規化routeで分析できる値を使う。Cookieや認証情報の生値を
access logへ保存しない。user-transition用の一例は、アプリが`X-Measurement-Session-ID` response headerで
認証情報とは別の仮名を返し、複数のAPP_HOSTSで同じsessionを同じ値へ対応づける方法である。
既存実装に別の安全な受け渡し方法があればそれを使い、識別列には集計用の値だけを出す。
`tools/contest/user-transition-routes.json`の
`cookie_field`と一致させる。API分類とroute正規化の設定方法は[`tools/user-transition-metrics/README.md`](../../tools/user-transition-metrics/README.md)を参照する。

nginxのJSON設定を変えたら、生成物の`nginx/conf.d/upstream.conf`には触れず、管理対象の設定を編集して
`task deploy-nginx`で反映する。`nginx -t`の成功と設定読込みを確認する。`task setup-smoke`を使うと、
ベンチを実行せずにaccess log・upstream集計・user-transitionの入力が作られることを確認できる。

空ログはtrafficがなかった状態として圧縮・保存される。必須列やJSONが不正な場合はRUNの成果物検査が失敗する。
識別情報を安全に出せない競技ではuser-transitionを無効化する理由を残す。詳細なアクセスログ回収・集計設定は
[tools/README.mdの「計測と集計」](../../tools/README.md#計測と集計)を参照する。

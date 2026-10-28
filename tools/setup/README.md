# 取得と構文検査

通常の入口は`Taskfile.yml`です。このディレクトリには、
大会ごとの`CONFIG_CHECK_COMMAND`へ組み込める構文検査の例を置きます。

## DB再作成前のpreflight

`task db-recreate`は最初に`task setup-preflight`を実行します。ローカルのrole、取得物、
主schema、`RESET_INPUTS`に宣言した追加資材を確認し、再作成計画をdry-runします。
`RESET_INPUTS`にはDB構築や`POST /initialize`が読む追加のローカルファイルを列挙してください。
追加資材が無い場合は`none`です。主schemaは`SQL_DIR/SQL_SCHEMA_FILE`として別途確認します。
取得した資材がdeploy対象に入っていることも確認してください。

## 新規実装の入力

このテンプレートでは初期アプリと内部設定を取得しない。`task setup`は空の実装先を用意し、旧アプリ・nginx・MySQL・systemd・sysctl設定の取得タスクは無効にする。
設計の根拠は公式の外部仕様・提供データ・公開APIの入出力・資源条件に限定する。データとともに旧プログラムやスキーマを読み込む手順は使わない。
新構成の設定・schema・初期化処理を作成してから、以下の配布・構文検査を行う。

## 配布先へ上書きして構文検査する

標準の`deploy-nginx`と`deploy-mysql`は、設定を配布先へ上書きした後、実サーバーのバイナリで
構文検査します。成功してからreload／restartします。includeやsymlinkを書き換えた検査用コピーは作りません。

設定uploadの`validate`がこの処理を指定します。

```yaml
- label: nginx configuration
  role: nginx
  local: nginx/
  remote: /etc/nginx/
  delete: true
  validate: sudo nginx -t
```

`validate`付きのuploadは配布先を退避し、転送または構文検査の失敗時にその配布先を復元します。
復元時には今回追加されたファイルも除去します。配布先自体がsymlinkの場合は、リンク先を誤って
変更しないよう転送前にエラーにします。内部のinclude・symlinkは通常の配置のまま検査します。
全uploadの検査が成功するまで、サービスを変更するactivationは開始しません。

構文検査だけを行う場合は`task config-check`と[config-check.yaml](config-check.yaml)を使えます。
新構成の検査内容を作成した後、`CONFIG_CHECK_COMMAND: 'task config-check'`と設定します。
成功した設定は配布先に残ります。この場合もサーバーのファイルを書き換えるため、進行中RUNや
同じ配布先を操作している別作業がないことを確認してください。
標準のnginx `tls/`とMySQL `debian.cnf`は取得・配布とも除外し、ホスト上の値を保持します。
ホスト別overlayを追加する場合は、共通設定の`delete: true`で消えない除外指定と、個別upload・検査を
通常deployとconfig-checkの両方へ揃えてください。

nginxは`NGINX_HOSTS`、MySQLは`MYSQL_HOSTS`が対象です。検査コマンドはそれぞれ`nginx -t`と
`mysqld --validate-config`です。MySQLのversionや起動時の`--defaults-file`が異なる場合は、
実サービスと同じ設定を読むコマンドへ調整します。アプリや追加サービスの検査も実構成に合わせます。

退避先はホスト上の`/tmp/isucon-deployctl-<ID>`で、通常終了時は削除します。転送または検査が失敗した
uploadだけを復元し、他の成功したuploadは戻しません。SSH切断・復元失敗時は退避先を確認して復旧します。
reload／restart後の動作不良は自動復元の対象外です。git上の旧設定を使い正規deployで戻してください。

設定の検査はサービスの動作確認とは別です。deploy後のrole・疎通検査と、ベンチによる確認も必要です。

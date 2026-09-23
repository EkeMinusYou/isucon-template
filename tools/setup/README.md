# 取得と構文検査

通常の入口は`Taskfile.yml`です。このディレクトリには、取得対象を絞る設定と、
大会ごとの`CONFIG_CHECK_COMMAND`へ組み込める構文検査の例を置きます。

## DB再作成前のpreflight

`task db-recreate`は最初に`task setup-preflight`を実行します。ローカルのrole、取得物、
主schema、`RESET_INPUTS`に宣言した追加資材を確認し、再作成計画をdry-runします。
`RESET_INPUTS`にはDB構築や`POST /initialize`が読む追加のローカルファイルを列挙してください。
追加資材が無い場合は`none`です。主schemaは`SQL_DIR/SQL_SCHEMA_FILE`として別途確認します。
取得した資材がdeploy対象に入っていることも確認してください。

## webappの取得対象

`SETUP_WEBAPP_EXCLUDES`はrsyncの`--exclude-from`へ渡すファイルです。
既定の[webapp-excludes.txt](webapp-excludes.txt)は従来と同じ`node_modules/`だけを除外します。
大会のソース配置を確認した後、生成される実行ファイルやビルドディレクトリを追加してください。
先頭の`/`は取得対象である`webapp/`を基準にします。例えば`/go/app`はその実行ファイルだけを除外します。
`vendor/`はオフラインビルドに必要なことがあるので、一律には除外しません。
SQL、初期化スクリプト、画像などの実行時依存は残してください。除外対象が後で必要になった場合は、
該当パターンを外して`task setup-webapp`を再実行します。除外しても既にローカルにあるファイルは削除しません。

## schemaが取得範囲外へのsymlinkの場合

通常のrsyncはsymlink自体を保存します。schemaのリンク先が`/usr/share/...`など取得範囲外なら、
取得後にローカルでリンク切れになるため、対象schemaだけを`--copy-links`で実体として取得します。
実サーバーでリンク先と内容を確認し、次のような行を既存の適切な`setup-*`へ追加してください。
`webapp/`全体に`--copy-links`を指定すると、意図しないリンク先まで取得するため避けます。

```yaml
# Example commands to append to setup-mysql after resolving the actual path.
- mkdir -p webapp/sql/schema
- '{{.RSYNC}} --copy-links {{.SSH_USER}}@{{.MYSQL_HOST}}:/usr/share/example/schema.sql webapp/sql/schema/schema.sql'
```

取得先を`SQL_DIR/SQL_SCHEMA_FILE`に設定し、`test -f webapp/sql/schema/schema.sql`と
`test ! -L webapp/sql/schema/schema.sql`で実体を確認します。取得だけではdeploy対象になりません。
アプリの初期化が参照する場合は、`deployments.yaml`にも必要な配置先へのuploadを追加してください。
これはschemaファイルの取得・配置であり、DBへの適用や初期化は実行しません。

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
当日の検査内容を調整した後、`CONFIG_CHECK_COMMAND: 'task config-check'`と設定します。
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

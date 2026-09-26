# 手順・設定ソース

競技中に参照する、複数の構成で再利用しやすい設定を管理する。各資料の値を適用し、
既存の設定ブロックやサービス構成に合わせて記述位置と反映先を決める。
アプリ・MySQL・nginxへの計測の組み込み方は [`docs/measurement/`](../measurement/README.md) に分けて管理する。

競技ルールやアプリケーションの正しい挙動については、必ず [`docs/official/`](../official/) の公式資料を
優先する。公式資料とこのディレクトリの説明が食い違う場合は、公式資料に従う。

## 秘伝のタレ

- [nginx の設定](nginx.md)
- [MySQL の設定](mysqld.md)
- [MySQL ドライバーのパラメーター補間](interpolate-params.md)
- [MySQL のファイルディスクリプタ上限](file-descriptor.md)
- [カーネルパラメーター](kernel-parameters.md)

## このリポジトリでの反映先

設定の編集先と反映対象は、`Taskfile.yml` 冒頭の役割定義に従う。サーバー上で直接編集せず、ローカルの
管理対象を編集して、対象を絞った正規 Task で反映する。

| 対象 | ローカルの主な編集先 | 正規 Task | 反映範囲・影響 |
| --- | --- | --- | --- |
| nginx | `nginx/nginx.conf`、`nginx/sites-enabled/` など | `task deploy-nginx` | `NGINX_HOSTS`。`nginx -t` 後に reload |
| nginx の upstream | `Taskfile.yml` の役割定義 | `task deploy-nginx` | `task gen` が `nginx/conf.d/upstream.conf` を生成。生成物は直接編集しない |
| MySQL | `mysql/mysql.conf.d/mysqld.cnf` | `task deploy-mysql` | `MYSQL_HOSTS`。MySQL を restart |
| MySQL の FD 上限 | `etc/systemd/system/mysql.service.d/limits.conf` | `task deploy-mysql` | `MYSQL_HOSTS`。daemon-reload と MySQL restart |
| カーネルパラメーター | `etc/sysctl.conf` | `task deploy-sysctl` | `ALL_HOSTS`。`sysctl -p` で即時適用 |
| アプリの DB 接続プール | `Taskfile.yml` の `APP_DIR` 配下にある DB 接続処理 | `task deploy` または `task deploy-app` | `APP_HOSTS`。アプリを再起動 |

`task deploy-all`は上記をまとめて反映するが、データベースを初期化しない。初期化が必要な競技では、
公式手順を確認して明示的な破壊的Taskを別途定義し、通常deployと混同しない。
役割を変更した場合だけ、`task deploy-all` の後に `task apply-roles` で旧役割のサービス停止まで反映する。
特定の設定だけを反映するときは、対象の `deploy-*` Task を使う。

## 適用の基本手順

1. [`docs/official/`](../official/) で競技ルール、再起動後のデータ保持、変更禁止対象を確認する。
2. 対象の設定を生成元へ反映し、同じ設定ブロックに属する項目をまとめる。
3. 対象の `deploy-*` Task で反映する。ベンチマーク実行中や、反映対象を確認できない状態では行わない。
4. `task status`、必要に応じて `task check-network`、各ページの実効値確認コマンドを実行する。

nginx・MySQL・systemd・sysctl の設定断片は、アプリやスキーマに依存しない範囲を扱う。特定のスキーマや
サーバー構成に依存する解決策・実装例は、[`docs/solutions/`](../solutions/README.md) に分けて管理する。

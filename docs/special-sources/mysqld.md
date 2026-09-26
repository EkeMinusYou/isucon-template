# MySQL（mysqld.cnf）

MySQL の設定。リポジトリでは `mysql/mysql.conf.d/mysqld.cnf` の `[mysqld]` セクションを管理し、
`task deploy-mysql` で `MYSQL_HOSTS` に配布する。この Task は MySQL を restart するため、反映中は一時的に
接続できなくなり、バッファプールも冷える。

バッファプール、ログ、接続上限は下記の値を設定する。レプリケーションやデータ保持の要件は
公式資料に従う。

## InnoDB のバッファと flush

```ini
[mysqld]
innodb_buffer_pool_size = 1GB
innodb_flush_log_at_trx_commit = 2
innodb_flush_method = O_DIRECT
```

- `innodb_buffer_pool_size=1GB` は InnoDB のデータページとインデックスをキャッシュする領域を
  1 GiB 確保する。MySQL 以外のサービスと同居する場合も、この領域を含めたメモリー使用量を確認する。
- `innodb_flush_log_at_trx_commit=2` はコミット時の flush 回数を減らす代わりに、OS・電源障害時に直近約1秒分の
  コミットを失う可能性がある。公式資料が求める再起動後のデータ保持や、アプリケーションの整合性チェックを
  満たすことを確認してから使う。
- `innodb_flush_method=O_DIRECT` はデータファイルについて OS のファイルキャッシュとの二重バッファを避ける
  方式である。利用中のファイルシステム・ストレージで動作することと、I/O 待ちが悪化しないことを確認する。

`innodb_buffer_pool_size` はバイト単位で、`innodb_flush_log_at_trx_commit` と `innodb_flush_method` は
MySQL の実効値を確認する。

## binary log

レプリケーション、point-in-time recovery（PITR）、binary log を利用するバックアップなどが不要で、競技の
構成上無効化してよい場合に限り、binary log を無効化する。

```ini
disable-log-bin = 1
```

レプリケーションやクラスタ構成を使う場合だけでなく、障害復旧や監査で binary log を必要とする場合も、
この設定を適用しない。`disable-log-bin` は設定ファイル上の起動オプションなので、実効状態は `log_bin` で確認する。

## 接続数とアプリケーションのプール

```ini
max_connections = 10000
```

`max_connections=10000` は MySQL が受け付ける同時クライアント接続数の上限であり、起動時に
10000接続を作る設定ではない。接続が実際に増えると接続ごとのメモリーを消費するため、メモリー使用量と
接続エラーを確認する。この上限だけでアプリケーションの並列度は増えない。

Go アプリケーションで `SetMaxOpenConns` を設定すると、MySQL の上限とは別にアプリ側の同時接続数を
制限する。この資料ではアプリ側に固定の接続上限を追加しない。既に上限がある場合は、その値による
プール待ちが発生することを確認する。

## 実効値の確認

```shell
sudo mysql -e "SHOW VARIABLES WHERE Variable_name IN (
  'innodb_buffer_pool_size',
  'innodb_flush_log_at_trx_commit',
  'innodb_flush_method',
  'log_bin',
  'max_connections'
);"
```

MySQL が active であること、期待した値が読み込まれていること、サービスメトリクスに異常が
ないことを確認する。設定だけを反映する場合は `task deploy-mysql` を使い、
初期化処理を別途定義している場合も、この設定の反映には使わない。

# MySQL（mysqld.cnf）

MySQL の設定例。リポジトリでは `mysql/mysql.conf.d/mysqld.cnf` の `[mysqld]` セクションを管理し、
`task deploy-mysql` で `MYSQL_HOST` に配布する。この Task は MySQL を restart するため、反映中は一時的に
接続できなくなり、バッファプールも冷える。

ここにある値は候補値であり、ホストのメモリー、データサイズ、書き込み量、レプリケーションの有無を
確認してから採用する。

## InnoDB のバッファと flush

```ini
[mysqld]
innodb_buffer_pool_size = 1GB
innodb_flush_log_at_trx_commit = 2
innodb_flush_method = O_DIRECT
```

- `innodb_buffer_pool_size` は InnoDB のデータページとインデックスをキャッシュする領域である。`1GB` は
  例であり、MySQL 以外のサービスが同居する場合も含めて、空きメモリーとワーキングセットに合わせる。
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

`max_connections` は MySQL が受け付ける同時クライアント接続数の上限である。大きくしすぎると、接続ごとの
メモリー使用量によってメモリー不足になる可能性がある。上限を増やすだけでアプリケーションの並列度が増える
わけではない。

アプリケーション側のDB接続プールも別に制御する。Go実装を採用した場合は、`Taskfile.yml` の
`APP_DIR` が指すソースからDB接続の初期化箇所を探し、たとえば次のように設定する。

```go
db.SetMaxOpenConns(50)
```

MySQL の `max_connections`、アプリの `SetMaxOpenConns`、同じ MySQL へ接続する別サービスの
接続数を合計して、実際の負荷とメモリー使用量に合わせて調整する。

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

MySQL が active であること、期待した値が読み込まれていること、slow query やサービスメトリクスに異常が
ないことを確認してから、同じ条件のベンチマークと比較する。設定だけを反映する場合は `task deploy-mysql` を使い、
初期化処理を別途定義している場合も、この設定の反映には使わない。

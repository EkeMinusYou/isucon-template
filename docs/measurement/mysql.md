# MySQLの計測データ

標準計測は各MySQL hostから毎秒のglobal status、slow query log、Performance Schemaのstatement digestとlock waitを取得する。
MySQLは`MYSQL_HOSTS`全台を対象にし、`MYSQL_HOST`だけを対象とする設定ではない。

## 取得元と前提

- **status** — collectorはUnix socket `/var/run/mysqld/mysqld.sock`で接続し、`SHOW GLOBAL STATUS`と`@@global.max_connections`を1秒間隔で読む。root socket接続と読み取り権限を確認する。
- **slow query log** — digest設定は`/var/log/mysql/mysql-slow.log`を回収して`pt-query-digest`へ渡す。MySQLの`slow_query_log`が有効で、実効`slow_query_log_file`がこのパスと一致することを確認する。別のパスなら`tools/measurectl/digesters.yaml`のsourceも合わせる。
- **statement digest** — `performance_schema`が有効な場合、`events_statements_summary_by_digest`をRUN前にリセットし、RUN後に各hostから集計する。無効な場合はdigestを取れないため、`@@performance_schema = 1`を確認する。
- **lock wait** — `performance_schema.data_lock_waits`、`data_locks`、`threads`、`events_statements_current`を50ms間隔で読む。DB versionで必要なtableが使えることと、collectorのroot socket接続を確認する。

slow logとPerformance Schemaの取得元を有効にする設定例:

```ini
[mysqld]
slow_query_log = ON
slow_query_log_file = /var/log/mysql/mysql-slow.log
log_output = FILE
performance_schema = ON
```

既存の`[mysqld]`へ統合し、同じsectionを重複させない。`long_query_time`は採取したいSQLとログ量に合わせて決める。

設定確認には次を使う。

```sql
SHOW VARIABLES WHERE Variable_name IN (
  'slow_query_log', 'slow_query_log_file', 'long_query_time',
  'log_output', 'performance_schema'
);
SELECT @@global.max_connections;
```

slow logの対象量は`long_query_time`や記録対象の設定で変わる。競技のquery量に合わせ、必要なSQLが残ることと
ログ出力量を確認して決める。collectorはstatusの参照、slow logのflush、Performance Schema digestのresetを行う。
RUN開始処理がデータを消去・ローテートするため、setup時に対象pathと動作を確認する。

成果物は`<host>-mysql-status.tsv`、`<host>-mysql-lock-waits.tsv`、`<host>-mysql-digest.tsv`、
`<host>-pt-query-digest.log`。lock waitのサンプルで`capture_error`が埋まる場合は、table対応や権限を調べる。
各hostを同じRUN・時間窓で比較し、status、statement digest、slow query log、lock waitの欠損を区別する。
collector設定と回収処理の詳細は[tools/README.mdの「計測と集計」](../../tools/README.md#計測と集計)を参照する。

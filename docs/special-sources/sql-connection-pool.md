# Go の DB 接続プール

Go アプリケーションの `database/sql`（`sqlx` を含む）で、DB 接続数の上限とアイドル接続の保持数を
設定する。事前の詳細計測を必須にせず、ISUCON の初動で初期候補を適用し、負荷時の結果から調整する。

## 設定

DB 接続を作成した後、リクエスト処理を始める前に設定する。

```go
const maxConns = 25
db.SetMaxOpenConns(maxConns)
db.SetMaxIdleConns(maxConns)
```

25 は [catatsuy 氏の ISUCON メモ](https://github.com/catatsuy/memo_isucon#goでmysqlのコネクションを管理する)を
参考にした初期候補であり、どの構成でも最適な値ではない。既に設定がある場合は重複追加せず、その値を調整する。

| 設定 | 意味・既定値 | 方針 |
| --- | --- | --- |
| `SetMaxOpenConns` | 使用中とアイドルを合わせた接続数の上限。既定値は0（無制限） | アプリ側の接続上限を明示する。上限に達すると新しいDB操作は接続待ちになる |
| `SetMaxIdleConns` | アイドル接続の保持数の上限。現在のGoの既定値は2 | 原則Openと同値にし、接続の作り直しを減らす。保持する接続の分だけ資源を使う |

上限は `sql.DB` ごとに適用される。同じ MySQL に接続する全アプリプロセス・全プールの Open 上限の合計が、
MySQL の `max_connections` を超えないようにし、初期化・管理・計測用の接続にも余裕を残す。
MySQL の接続上限は [MySQL の設定](mysqld.md)を参照する。

Idle を Open より大きく指定しても、正の Open 上限まで自動的に制限されるため、同値でよい。
どちらも接続を設定数まで事前に作成する設定ではない。

## 適用後の調整

スコアと以下の指標を、同じ RUN・観測窓で比較する。累積カウンターは観測窓の差分を使い、
RUN 間で時間や処理量が違う場合は秒あたり・処理あたりでも比較する。

| 確認する指標 | 調整の判断 |
| --- | --- |
| `InUse`、`WaitCount`、`WaitDuration` と MySQL の CPU・`Threads_running` | Open上限付近で待ちが増え、DBに余力があるならOpenの増加を試す。DBが飽和しているなら待ちだけを理由に増やさない |
| `MaxIdleClosed` と MySQL の新規接続数（`Connections`） | Idle上限による切断と新規接続が多いなら、保持数の増加を試す |
| アプリ・MySQLのメモリー、接続エラー | 接続増加による資源消費や接続上限超過がないか確認する |

計測方法は [SQL接続プールの計測](../../tools/measurement/docs/sql-pool-metrics.md)を参照する。
このリポジトリでは既存の RUN 成果物と `task q -- "SQL"` で比較し、接続数設定のために計測基盤を追加しない。
既存設定の調整では、OpenとIdleの効果を区別できるよう、原則1回のRUNで片方ずつ変更する。

## 反映

`Taskfile.yml` の `APP_DIR` 配下の Go 実装を編集し、`task deploy` または `task deploy-app` でビルド・反映する。
いずれも `APP_HOSTS` のアプリを再起動するため、
実行前に対象ホストを確認し、反映後は `task status` とアプリケーションログを確認する。

## 参考資料

- [catatsuy 氏の ISUCON メモ](https://github.com/catatsuy/memo_isucon#goでmysqlのコネクションを管理する)：Openを25程度から調整する初期候補。
- [go-sql-driver/mysql の推奨設定](https://github.com/go-sql-driver/mysql#important-settings)：Openの設定とIdleを同値にすることを推奨。具体的な上限値はアプリとMySQL次第。
- [Go公式の接続管理](https://go.dev/doc/database/manage-connections)：各設定の挙動と `DB.Stats` による効果の確認。
- [ISUCON13参加記](https://trap.jp/post/2075/)：DB・HTTPクライアントの接続数調整を「アプリの秘伝のタレ」として実施した例。

# 計測の組み込み

既存のアプリケーション、MySQL、nginxから計測データを出すための設定・実装手順をまとめる。
collectorや集計ツール自体の説明は各ツールのREADMEと[tools/README.md](../../tools/README.md)に置く。

## 一覧

- [Go profileの自動収集](go-profiling.md) — アプリへpprof・fgprof・RUN別開始確認endpointを組み込む
- [nginx access logの計測項目](nginx-access-log.md) — 標準集計に必要なJSON列とセッション識別情報を設定する
- [MySQLの計測データ](mysql.md) — status、slow query、Performance SchemaからRUN単位のデータを取得する

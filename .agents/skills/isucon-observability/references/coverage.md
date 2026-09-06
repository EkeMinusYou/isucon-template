# 計測の確認項目

既定設定を実環境の事実とみなさず、現行コード・実効設定・成果物を照合する。詳細な列定義と設定項目は[Tools](../../../../tools/README.md)を正本とする。

| 対象 | 確認する経路と不足 |
| --- | --- |
| nginx access log | 実際のtrafficを受けるserver/locationの`access_log`と継承・無効化、JSONのescape、出力先・権限、buffer/flush、ローテート後のreopenを確認。共通列とupstream列がToolsの契約を満たし、alp・upstream集計まで読めるかを見る。正常なHTTP応答だけでログ出力を確認済みにしない。 |
| Go pprof・fgprof | endpointの実装・handler登録と実際のlistener、collectorの取得URL・実行ホスト、既定無効のoneshotと起動経路を確認。`Taskfile.yml`にURLがあるだけでは公開済みとしない。CPU・heap・allocs・goroutine・fgprofを目的別に選び、取得ファイルを`go tool pprof -top`等で解析する。 |
| MySQL | slow logの実効設定・閾値・出力先・ローテートとdigest、performance_schemaの有効性・権限・version、status・接続・lock waitを確認。`MYSQL_HOSTS`と詳細計測先`MYSQL_HOST`の差による未被覆を残す。空のslow logは低負荷・閾値未満・収集失敗を区別する。 |
| ホスト・service | 全対象ホストのCPU・メモリ・disk・ネットワークと、実際のunit/process/cgroupの対応を確認。追加serviceや別ホストのDBも対象に含める。欠損、counter reset、単位、sampling間隔を確認する。 |
| 障害・整合性 | app/nginxのjournal・error log、kernel/OOM、panic、service再起動が同じload windowで追えるか確認する。正常系のprofileだけで網羅済みにしない。 |
| その他の依存サービス | 競技ごとの公式資料と稼働serviceから依存経路を列挙する。既存service metrics・journalで見える範囲と、処理件数・失敗・遅延など追加観測が必要な範囲を分ける。 |
| アプリ内部・optional計測 | DB pool待ち、queue、処理件数などは既存profile・標準計測で説明できない問いがある場合に追加する。ユーザー遷移、task-state、lock wait、nginx on-CPUも目的・負荷・停止方法を確認して選ぶ。 |
| RUN全体 | load windowと各計測の開始・終了、ホスト時計、取得遅延・欠落、roles/source、artifact status、rawから集計・分析への対応を確認。ヘッダーだけのfallbackと実測値を区別する。 |

## Go profileを整えるとき

- 計測用listenerはloopbackなど必要な範囲へ限定し、公開traffic用routerへ無条件に登録しない。remote curlがどのホストで動くかとbind先を照合する。
- 標準pprofとfgprofは別のendpoint・実装として確認する。handler未登録、timeout、HTTPエラー本文の保存を取得成功に数えない。
- `PROFILE_DELAY`、`SNAPSHOT_PROFILE_DELAY`、取得秒数、timeoutと実際の起動時刻を照合する。負荷終了後に手動タスクを呼んでも負荷中のprofileにはならない。
- 同じプロセスへのCPU profile取得が重複しないようにする。fgprofとの同時利用も実装・負荷を確認する。block/mutexのような追加profileはsampling設定と負荷を確認してから使う。
- CPU時間、wall-clock、heapの時点値、allocsの累積値を区別する。profileに紐づくソース・binaryと採取時間を追えるようにし、空または疎なprofileの疎通成功から負荷中の有用性を断定しない。

## ログ・追加メトリクスを整えるとき

- 形式・列名・時刻・単位を生成側とparserで一致させる。URIの動的IDは集計側で正規化し、upstreamがない応答や複数upstreamへの試行も扱えることを確認する。
- Cookie・認証情報・ユーザーIDを安易に追加しない。ユーザー遷移の識別列はToolsの取り扱いに従い、目的がなければ無効のままとする。
- 追加計測はsampling間隔、対象、timeout、出力量・cardinality、enable/disableとcleanupを定める。計測を増やした結果、CPU・I/O・disk容量を圧迫しないか検証する。

# 標準計測の確認手順

既定設定を実環境の事実とみなさず、現行コード・実効設定・成果物を照合する。詳細な列定義と設定項目は[Tools](../../../../tools/README.md)を正本とする。

| 対象 | 確認する経路と不足 |
| --- | --- |
| スコア記録 | 公式の採点仕様と出力形式を確認し、生成元、取得・手動入力の手順、RUNへの紐付け、保存先・形式を整える。最終スコア、成功・失敗判定、公式出力にある内訳・ペナルティが元の結果と一致し、DuckDB・dashboardまで読めることを確認する。未取得・未判定を0点や成功に置き換えない。 |
| nginx access log | 実際のtrafficを受けるserver/locationの`access_log`と継承・無効化、JSONのescape、出力先・権限、buffer/flush、ローテート後のreopenを確認。共通列とupstream列がToolsの契約を満たし、alp・upstream集計まで読めるかを見る。正常なHTTP応答だけでログ出力を確認済みにしない。 |
| Go pprof・fgprof | CPU・heap・allocs・goroutine・fgprofの5種類を標準として整備。endpointの実装・handler登録と実際のlistener、collectorの取得URL・実行ホスト、有効宣言と起動経路を確認。`Taskfile.yml`にURLがあるだけでは公開済みとしない。取得ファイルを`go tool pprof -top`等で解析する。 |
| MySQL | slow logの実効設定・閾値・出力先・ローテートとdigest、performance_schemaの有効性・権限・version、status・接続・lock waitを確認。`MYSQL_HOSTS`と詳細計測先`MYSQL_HOST`の差による未被覆を残す。空のslow logは低負荷・閾値未満・収集失敗を区別する。 |
| ホスト・service | 全対象ホストのCPU・メモリ・disk・ネットワークと、実際のunit/process/cgroupの対応を確認。追加serviceや別ホストのDBも対象に含める。欠損、counter reset、単位、sampling間隔を確認する。 |
| 障害・整合性 | app/nginxのjournal・error log、kernel/OOM、panic、service再起動が同じload windowで追えるか確認する。正常系のprofileだけで網羅済みにしない。 |
| その他の依存サービス | 競技ごとの公式資料と稼働serviceから依存経路を列挙する。既存service metrics・journalで見える範囲と、処理件数・失敗・遅延など追加観測が必要な範囲を分ける。 |
| user-transition | 標準対象。アプリのセッション仕様に合わせた識別列と有限個のAPI分類を整える。実ログに識別列があり、認証済みリクエストを結び付けられ、集計とdashboardの遷移表示に反映されることを確認する。未設定はoptionalではない。 |
| アプリ内部・追加計測 | DB pool待ち、queue、処理件数などは既存profile・標準計測で説明できない問いがある場合に追加する。高頻度task-state、lock wait、nginx on-CPUも目的・負荷・停止方法を確認して選ぶ。 |
| RUN全体 | load windowと各計測の開始・終了、ホスト時計、取得遅延・欠落、roles/source、artifact status、rawから集計・分析への対応を確認。ヘッダーだけのfallbackと実測値を区別する。 |

## スコア記録を整えるとき

- 既存のTaskfileと記録経路を優先し、自動取得か手動入力かを含め、ユーザーがベンチ結果を同じRUNへ記録できる手順を明確にする。ベンチ自体は代理実行しない。
- 競技固有の出力へ対応する場合は保存するmarker形式を先に定め、[Toolsの分析契約](../../../../tools/README.md#分析)に従って保存側と読み手を揃える。既存の`scores.tsv`形式を維持する。
- `tools/analysis/sources.yaml`と既存schema・意味ビューを確認し、スコアや判定・内訳の意味を既存定義で表せない場合は、取り込みschemaに加えて意味ビューも対応する。必要なquery・dashboardのAPIと表示まで整合させる。
- 保存済みRUNがあれば元のベンチ結果と保存値・分析結果・表示を照合する。失敗結果やスコア欠損を区別し、実負荷での確認が必要ならユーザーへ次RUNを依頼する。既存RUNは上書きしない。

## Go profileを整えるとき

[Go profile導入例](../../../../docs/special-sources/go-profiling.md)のadapterと開始確認の契約を参照する。

- 計測用listenerはloopbackなど必要な範囲へ限定し、公開traffic用routerへ無条件に登録しない。remote curlがどのホストで動くかとbind先を照合する。
- 標準pprofとfgprofは別のendpoint・実装として確認する。handler未登録、timeout、HTTPエラー本文の保存を取得成功に数えない。
- `PROFILE_DELAY`、`SNAPSHOT_PROFILE_DELAY`、取得秒数、timeoutと実際の起動時刻を照合する。負荷終了後に手動タスクを呼んでも負荷中のprofileにはならない。
- CPUとfgprofは初期化・チェック・負荷・終了処理と余裕を覆う時間、snapshotは負荷中になる遅延を設定する。特定の秒数を競技共通の保証値としない。通常ベンチが採取開始を確認し、回収完了を待ってfinalizeすることを確認する。
- 同じプロセスへのCPU profile取得が重複しないようにする。fgprofとの同時利用も実装・負荷を確認する。block/mutexのような追加profileはsampling設定と負荷を確認してから使う。
- CPU時間、wall-clock、heapの時点値、allocsの累積値を区別する。profileに紐づくソース・binaryと採取時間を追えるようにし、空または疎なprofileの疎通成功から負荷中の有用性を断定しない。

## ログ・追加メトリクスを整えるとき

- 形式・列名・時刻・単位を生成側とparserで一致させる。URIの動的IDは集計側で正規化し、upstreamがない応答や複数upstreamへの試行も扱えることを確認する。
- user-transitionはセッションを安定して識別できる方法を調査し、認証Cookie等の生値を恒久保存しない識別列を出す。ログ列名とroutes.jsonのcookie_fieldを一致させ、集計成果物に識別値を残さない。識別方法が仕様上成立しない場合のみ理由を示して対象外とする。
- routes.jsonの汎用プレースホルダーをそのまま完成扱いにしない。公式APIとGoのroute登録から動的IDを正規化し、未分類API・識別列欠損・時刻不正の件数も確認する。
- nginxのmsec等、実際の出力形式をDuckDBとdashboardのparserまで照合する。圧縮ログ・空ログ・不正入力を確認し、UIにデータがない場合は未取得・空・解析失敗・設定無効を切り分ける。
- 追加計測はsampling間隔、対象、timeout、出力量・cardinality、enable/disableとcleanupを定める。計測を増やした結果、CPU・I/O・disk容量を圧迫しないか検証する。

## 有効宣言と対象外判断

計測の有効宣言や対象範囲を整備・補修するときに確認する。

有効対象の正本はcollector/digesterの`enabled_by_default`。profileは`group: profiles`の有効な宣言を
自動収集・開始確認・必須成果物判定で共通利用する。`PROFILES_ENABLED=false`は比較等での明示的な停止用であり、
未設定を隠すために使わない。宣言と自動実行対象の名前一覧を別々に維持しない。

仕様上成立しない計測は根拠を示して対象外にできる。たとえば安定したセッションを識別できないアプリでの
user-transitionは、代替の識別方法も確認したうえで理由を記録する。高頻度task-state・lock wait、
nginx on-CPU、block/mutex、アプリ内部状態などの追加計測は目的・負荷・停止方法を確認して選ぶ。

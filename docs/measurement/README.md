# 計測の組み込み

Goアプリケーションとホストから計測データを出すための設定・実装手順をまとめる。
collectorや集計ツール自体の説明は各ツールのREADMEと[tools/README.md](../../tools/README.md)に置く。

## 一覧

- [nginxアクセスログ](nginx-access-log.md) — 公開入口の要求・操作遷移・負荷期間の記録
- [Goアクセスログ](go-access-log.md) — Go内部の追加診断用
- [Go profileの自動収集](go-profiling.md) — アプリへ標準pprof・RUN別開始確認endpointを組み込む。fgprofは任意
- [状態・操作の計測](state-and-operations.md) — 永続化方式によらない件数・進行・待ち時間の確認と集計の保存

WALを採用した場合だけ、[WALの分析と保存](wal-analysis.md)も参照する。

## 標準計測で答えられる問い

DuckDBの分析値は`load_windows`の負荷期間に限定する。入口のtrafficから候補区間を検出し、リクエスト数が最大の区間を選ぶ。期間は推定であり、
公式出力に負荷開始・終了がある場合は`load_phase`設定で指定できる。期間を決められないRUNは分析値を取り込まない。
採点結果・設定・回収状態などのメタデータは期間外も保持し、成果物はRUNに残す。

| 計測 | 解像度・対象 | 用途と限界 |
|---|---|---|
| ホスト・service・Go runtime | 1秒 | 資源量・処理量・待ちの推移。短い変動は取り逃がし、counterの差分は観測間隔に対応する |
| heap・goroutine profile | 2秒 | 保持メモリ・stackの増減。短い待ちの時間計測には使わない |
| access log | 要求ごと→最大5秒のbucket | 負荷期間内に開始した要求を先に選ぶ。境界bucketは実際の秒数を保持する。HTTP時間はCPU時間ではない |
| CPU・任意fgprof | 採取全体 | 関数別のCPU等。初期化を含むため負荷期間のみのDB値から除外する |
| alp・user-transition | 採取全体の集計 | 元のRUN成果物で全体を読む。期間を分離できずDB取り込みを除外する |
| journal・error log | 回収期間 | 障害の原文。負荷期間へ分離したDB値は作らずRUNで読む |

除外理由は`measurement_sources`と`go_profile_import_scope`で確認でき、`compare-runs`にも表示する。
空の分析表を「処理なし」「CPU負荷なし」と解釈しない。全体のuser-transition等が必要な場合はRUNのJSONを読む。

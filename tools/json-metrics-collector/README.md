# JSON snapshot metrics collector

アプリがlocalhostに公開するbounded JSON snapshotを定期取得し、long-form TSVへ変換する汎用collectorです。
開始時にenable endpoint、走行中にsnapshot endpoint、終了時にdisable endpointを呼びます。

標準計測には組み込まれていません。当日のアプリへ計測endpointを実装し、
`tools/measurectl/collectors.yaml`へ`enabled_by_default: false`のcollectorを追加して使います。

```shell
./json-metrics-collector \
  -enable-endpoint http://127.0.0.1:6060/debug/metrics/enable \
  -snapshot-endpoint http://127.0.0.1:6060/debug/metrics \
  -disable-endpoint http://127.0.0.1:6060/debug/metrics/disable \
  -interval 1s -output metrics.tsv
```

入力snapshotはversion、generation、enabled、captured time、metricsを持ち、各metricは
scope、entity_id、group_id、window_start_ms、name、valueを持ちます。responseとoutputにはbyte limitがあり、
値の意味はアプリ側が所有します。個人情報や高cardinalityの識別子を成果物へ出さないでください。

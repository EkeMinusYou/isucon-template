# topology-screen

保存済みnginx access logをオフラインで再生し、任意のキーをhash shardへ割り当てた場合の
リクエスト数、response time、upstream time、body bytesの分布を比較します。稼働中のnginxや
役割構成は変更しません。

```shell
task topology-screen -- -moduli 2,3,4,5
task topology-screen -- -key-field tenant_id -key-regex '^(.+)$' -route-regex '^/api/'
task topology-screen -- -key-field vhost -key-regex '^([^.]+)\.example\.com$'
```

既定はJSON logの`vhost`全体をキーにします。問題固有のtenantやuser IDを取り出す場合は、
capture groupを1つ持つ`-key-regex`を指定してください。`.zst`は`zstdcat`、`.gz`は標準ライブラリ、
それ以外は平文として読みます。

主なflag:

- `-run-dir` / `-input` / `-input-glob` — 入力
- `-format json|ltsv` — log形式
- `-key-field` / `-key-regex` / `-key-exclude` — shard key
- `-hash fnv1a|fnv1|crc32|sha256` — hash
- `-moduli` / `-shard-buckets` — 比較する分割数と配置bucket
- `-route-regex` / `-normalize` — 対象routeと正規化
- `-field-*` — log列名

TSVをstdout、coverage summaryをstderrへ出します。キーやrouteが1件も一致しない場合、
空結果を「負荷なし」と誤認しないよう失敗します。

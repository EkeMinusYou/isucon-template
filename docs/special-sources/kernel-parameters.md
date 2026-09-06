# カーネルパラメーター

ネットワークの待ち行列や送受信バッファの上限を調整する設定例である。アプリケーション側の backlog、
ソケット設定、負荷が小さい場合の性能を自動的に改善するものではない。

## 設定と反映

リポジトリの管理対象である `etc/sysctl.conf` に以下を設定する。`etc/systemd/sysctl.conf` ではない。

```ini
net.core.somaxconn=65535
net.ipv4.ip_local_port_range=10000 60999
net.core.rmem_max=16777216
net.core.wmem_max=16777216
```

反映は `task deploy-sysctl` で行う。この Task は `ALL_HOSTS` へ `/etc/sysctl.conf` を配布し、各ホストで
`sysctl -p` を実行する。通常は再起動を必要としないが、対象カーネルに存在しないキーがある場合は適用に
失敗するため、エラーを無視して先へ進まない。

## 各項目の意味

| 項目 | 役割 | 注意点 |
| --- | --- | --- |
| `net.core.somaxconn` | `listen(2)` の accept 待ち行列のカーネル側上限 | アプリケーションが指定した backlog の上限を超えて増やすことはできない |
| `net.ipv4.ip_local_port_range` | 外向き TCP 接続に使う一時ポートの範囲 | nginx から app、app から MySQL などの発信接続に関係する。待受ポートの範囲ではない |
| `net.core.rmem_max` / `net.core.wmem_max` | ソケットの受信・送信バッファの最大値 | 最大値を引き上げるだけで、各ソケットがその量を確保するわけではない |

上記の数値は候補値であり、ホストのメモリー、接続数、`TIME_WAIT` 数、ネットワークエラーなどを計測して
採否を判断する。

## 実効値の確認

全ホストで対象項目を確認する。`ip_local_port_range` は二つの値が出力される。

```shell
for key in \
  net.core.somaxconn \
  net.ipv4.ip_local_port_range \
  net.core.rmem_max \
  net.core.wmem_max; do
  printf '%s=' "$key"
  sysctl -n "$key"
done
```

ホスト間で値が揃っていることと、`task status` で対象サービスが正常であることを確認してから、ベンチマーク
結果を比較する。

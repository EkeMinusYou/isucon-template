# MySQL のファイルディスクリプタ上限を増やす

ファイルディスクリプタは、ソケット、ログ、ファイルなどをプロセスが同時に開くためのハンドルである。
この設定は MySQL の systemd サービスに属する各プロセスの上限を増やすもので、`max_connections` を増やしたり、
実際の接続数を増やしたりする設定ではない。

## 設定と反映

`etc/systemd/system/mysql.service.d/limits.conf` に以下を設定する。

```ini
[Service]
LimitNOFILE=1006500
```

`1006500` はこのリポジトリで使用している値の例であり、全環境に必要な値ではない。接続数やログの量、
メモリー使用量を確認したうえで、過大な値を機械的に採用しない。

反映には `task deploy-mysql` を使う。この Task は設定ファイルを `MYSQL_HOST` へ配布し、
`daemon-reload` の後に MySQL を restart する。バッファプールが温まる前に一時的な性能低下が起こり得るため、
ベンチマーク実行中には反映しない。

この設定だけを試す目的では、他サービスも再反映する `deploy-all` ではなく `deploy-mysql` を使う。
初期化処理を別途定義している場合も、この設定の反映には使わない。

## 実効値の確認

ファイルの内容ではなく、systemd と稼働中プロセスの値を確認する。

```shell
sudo systemctl show mysql -p LimitNOFILE --value

mysql_pid=$(sudo systemctl show mysql -p MainPID --value)
sudo awk '/Max open files/ {print}' "/proc/${mysql_pid}/limits"
```

systemd の値とプロセスの `Max open files` が意図した値になっていること、MySQL が active であることを
確認してから、接続数や MySQL の計測結果を比較する。

# ファイルディスクリプタ上限

Goアプリのsystemd unitは`task gen`で`LimitNOFILE=65536`を設定します。公開接続の初期上限は1プロセス32768です。接続に加えてupstreamのidle接続、ログ、永続ファイルのFDも見積もります。これらは初期値であり、全競技の最適値ではありません。

変更は生成元のTaskfileへ反映し、`task deploy`でunitを配布・再起動します。`systemctl show <service> -p LimitNOFILE`と稼働プロセスの`/proc/<pid>/limits`を照合します。上限変更だけで接続数を無制限に増やさず、メモリ、接続待ち、CPUを確認します。

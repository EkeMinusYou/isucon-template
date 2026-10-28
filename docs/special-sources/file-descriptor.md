# ファイルディスクリプタ上限

接続・ログ・永続ファイルでFDが不足する場合、Goアプリのsystemd unitに`LimitNOFILE`を設定します。必要数を同時接続と開くファイル数から見積もり、OSの上限も確認してください。

変更はローカルのunitへ反映して`task deploy`で配布・再起動します。`systemctl show <service> -p LimitNOFILE`と実プロセスの`/proc/<pid>/limits`を照合します。上限変更だけで接続数を無制限に増やさず、メモリとバックプレッシャーも確認します。

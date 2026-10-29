# ファイルディスクリプタ上限

接続・ログ・永続ファイルでFDが不足する場合、Goアプリのsystemd unitに`LimitNOFILE`を設定します。必要数を同時接続と開くファイル数から見積もり、OSの上限も確認してください。

変更はローカルのunitへ反映して`task deploy`で配布・再起動します。`systemctl show <service> -p LimitNOFILE`と実プロセスの`/proc/<pid>/limits`を照合します。上限変更だけで接続数を無制限に増やさず、メモリとバックプレッシャーも確認します。

bootstrapで`etc/systemd/system/nginx.service.d/limits.conf`へ`[Service]`節と`LimitNOFILE=65536`を新規作成し、`task deploy-nginx`で配布します。nginx workerの上限16384と接続枠8192は`nginx/nginx.conf`で管理します。systemdの新しいlimitはサービス再起動後に反映されるため、`deploy-nginx`は稼働中masterと設定のsoft limitが異なる場合に再起動します。一致する場合はreloadです。反映後はmasterとworkerの`/proc/<pid>/limits`を確認します。

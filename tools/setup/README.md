# 新規実装の入力と検査

setupは初期ソースをリポジトリ外の`REFERENCE_DIR`へ取り出し、外部仕様と独立した初期データを用意します。別セッションのbootstrapが、汎用Go HTTP土台へ競技固有の実装を加えます。

`task initialize`は`setup-preflight`でホスト・生成したGo unit・ホスト別HTTP設定・初期化資材・配布計画を確認してから、Goの初期化APIを呼びます。`RESET_INPUTS`は全初期化ファイル、不要なら`none`です。配布対象にも含め、永続化先と混同しません。

`CONFIG_CHECK_COMMAND`はローカルのGo・設定検査です。既定は`go vet`です。`setup-check`は必要ツール・秘密情報・build・テスト・成果物契約・配布dry-runも確認します。実環境の配布物・役割・通信は`setup-postflight`、正当性と実負荷の計測は`task bench`で確認します。

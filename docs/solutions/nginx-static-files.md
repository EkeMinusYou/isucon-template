# 公開静的ファイルをGoから配信する

公開不変ファイルは`STATIC_DIR`（既定`public/`）から`STATIC_REMOTE_DIR`へ配布し、`STATIC_URL_PREFIX`で宣言したURLからGoが直接返す。`task deploy-all`または`task deploy-static`を使う。既定のcache・gzip・validatorは[秘伝のタレのGo対応](../special-sources/nginx.md)を参照する。

同じURLで更新する画像・生成物・認可付きファイルは、不変資材の配信へ混ぜない。公式の更新・削除・初期化・通常再起動後の可視性に合わせ、Goのhandlerで認可・validator・cache無効化・必要なホスト間同期を実装する。永続領域は配布時の削除対象外に置く。

静的内容のcacheを再生成する際はプロセスの通常restartを行う。即時の更新反映が必要なURLは競技側で無効化経路を用意する。APIをURLの推測でファイル配信へ切り替えない。

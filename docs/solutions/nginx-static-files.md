# 静的データをファイル化してnginxから配信する

## 概要

JavaScript・CSS・画像・生成済みコンテンツの本体配信をnginxへ任せ、Goの業務処理・ファイル読み出し・応答生成を減らす。クライアントのキャッシュ期限も設定すれば、再取得の要求自体を減らせる。

## 適用条件・制約

公開範囲・更新・削除・初期化後の可視性を維持する。認証が必要なデータを公開locationへ置かない。URLにcontent hashや世代を含めると不変データの長期キャッシュが使いやすい。同一URLで上書きするデータは失効・validatorを設計する。複数nginxホストでは同じURLの内容が一致する配布・同期方法が必要になる。

## 探索方法

外部仕様・公開APIと提供資材から、固定のフロントエンド資材、画像・添付の取得API、大きな応答を探す。Go実装の`ServeFile`・`io.Copy`・`Content-Type`等とnginxの`location`・`root`・`alias`を対応付け、認可と更新が必要な経路を分ける。初期ソース・旧DBスキーマを読むための例外にはしない。

## 実装方法

固定の公開資材は`STATIC_DIR`（既定`public/`）から`task deploy-nginx`で全NGINX_HOSTSへ配布する。たとえば`public/assets/`の不変ファイルを`/assets/`で公開する場合、`nginx/sites-enabled/app.conf`へ次を加える。

```nginx
location /assets/ {
    include /etc/nginx/snippets/static-cache.conf;
    try_files $uri =404;
}
```

rootは`task gen`が`STATIC_REMOTE_DIR`から生成する。これはURL・更新条件を確認した場合の例で、全APIへ適用しない。具体的な期限・圧縮設定は[nginxの秘伝のタレ](../special-sources/nginx.md)を参照する。

実行中のアップロード・生成物は配布時の削除対象である`STATIC_DIR`と別の永続領域へ置く。検証済みIDやcontent hashからパスを作り、一時ファイルを完成させてからrenameし、メタデータと公開順序を揃える。複数ホストへの可視化と通常再起動後の復元もGo側で担う。

認可付きファイルはGoで認可を済ませてから`X-Accel-Redirect`でnginxの`internal` locationへ委譲できる。保存領域を直接公開せず、キャッシュ方針も認可・失効条件に合わせる。

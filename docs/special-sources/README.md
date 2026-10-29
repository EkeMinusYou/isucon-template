# 秘伝のタレ：Go HTTP・OS・実行環境

Goのみの基本設定です。[適用スキル](../../.agents/skills/isucon-special-sauce/SKILL.md)で初回ベンチ前に統合します。nginxの秘伝のタレが担っていた役割をGoの汎用HTTP土台へ移しています。値と適用条件を確認し、公式仕様・更新時の正当性を優先します。

- [Go HTTP：接続再利用・TLS・静的キャッシュ・事前圧縮](go-http.md)
- [ファイルディスクリプタ上限](file-descriptor.md)
- [カーネルパラメーター](kernel-parameters.md)

`task gen`でHTTP設定・静的資材・unitを生成し、`task deploy`または`task deploy-all`で配布します。通常deployでは状態を初期化しません。nginxを含む不要サービスは停止・無効化します。

# 秘伝のタレ：nginx・OS・実行環境

Go単体構成の基本設定です。[適用スキル](../../.agents/skills/isucon-special-sauce/SKILL.md)で初回ベンチ前に統合します。値と適用条件を確認し、公式仕様・更新時の正当性を優先します。DB関連のレシピはこの構成には含めません。

- [Goの公開入口：nginxレシピから接続再利用・静的キャッシュ・圧縮を移植](nginx.md)

- [ファイルディスクリプタ上限](file-descriptor.md)
- [カーネルパラメーター](kernel-parameters.md)

ローカルの`etc/`へ設定し、`task deploy-sysctl`またはGoアプリのunit配布で反映します。`task deploy-all`は不要サービス停止とGoアプリ・静的資材・TLS配布も行います。通常deployでは状態を初期化しません。

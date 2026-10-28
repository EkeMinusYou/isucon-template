# OS・実行環境の設定資料

Goアプリを動かすOS・systemdの設定候補です。値を自動適用せず、資源と実測から選んでください。公式資料を優先します。

- [ファイルディスクリプタ上限](file-descriptor.md)
- [カーネルパラメーター](kernel-parameters.md)

ローカルの`etc/`へ設定し、`task deploy-sysctl`またはGoアプリのunit配布で反映します。`task deploy-all`は既製ミドルウェア停止とGoアプリ配布も行います。通常deployでは状態を初期化しません。

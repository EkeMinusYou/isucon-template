# MySQL ドライバーのパラメーター補間

アプリケーションの DB 接続設定で、MySQL ドライバーによるパラメーター補間を有効にする設定である。
MySQL サーバーの設定ではなく、Go アプリケーション側の接続設定に追加する。

## 設定

`mysql.NewConfig()` で作成した接続設定に、次の値を設定する。

```go
config.InterpolateParams = true
```

このリポジトリでは、`webapp/go/main.go` の `connectDB` にある接続設定へ追加する。
実装上の変数名が `conf` の場合は、`conf.InterpolateParams = true` と記述する。

## 反映

Go バイナリをビルドしてアプリケーションへ反映するため、`task deploy` または `task deploy-app` を使う。
反映後は `task status` とアプリケーションログを確認し、同じ条件のベンチマークで性能と整合性を比較する。

この設定だけを試す場合は `task deploy` または `task deploy-app` を使い、
初期化処理を別途定義している場合も、この設定の反映には使わない。

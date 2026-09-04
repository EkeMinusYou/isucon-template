# Performance

原点0からの絶対量とcapacityを比較する。割合だけで律速を決めない。

- response time、CPU実仕事、I/O、lock・queue wait、DB query timeを分ける。
- 同じ単位・母数・時間窓で候補を比較する。
- 集約総額と、一つのChange boundaryが実際に削る量を分ける。
- cache、batch、非同期化、往復削減、不要な仕事、admission制御を候補源として確認する。
- 別hostへの移設は、移設後capacityと新規network・serializationコストを含める。

Constraintを`RESOLVES`するときだけ、現在値 − 境界削減 + 追加コスト = 予測残差を解消条件と比較する。届かない正方向変更は`MITIGATES`であり、無価値ではない。

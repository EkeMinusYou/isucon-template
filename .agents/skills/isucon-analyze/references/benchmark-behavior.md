# Benchmark behavior

bench.log、access log、ユーザー遷移集計、公式仕様から、事実と推論を分けてシナリオを読む。

- open-loopかclosed-loopか、次操作が何の完了を待つか
- retry、timeout、終了時切断、admissionの条件
- Cookieやactor別の系列、シナリオ間の競合
- 応答短縮が次の得点行動を増やす経路

観測されない内部実装を断定しない。同じ現象を説明する代替仮説と、現在Evidenceで区別できる範囲を残す。

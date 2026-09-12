# Performance

原点0からの絶対量とcapacityを比較する。割合だけで律速を決めない。

- response time、CPU実仕事、I/O、lock・queue wait、DB query timeを分ける。
- 同じ単位・母数・時間窓で候補を比較する。
- 集約総額と、一つのChange boundaryが実際に削る量を分ける。
- cache、batch、非同期化、往復削減、不要な仕事、admission制御を候補源として確認する。
- 別hostへの移設は、移設後capacityと新規network・serializationコストを含める。

Targetの今回の目標と評価条件を確認する。容量対策では現在値・削減・追加コスト・移設先の余力を同じ単位で検討するが、起票に残差計算や既知の効果量を必須としない。十分速い要求の往復削減も対象にする。確認した機構、推定する改善方向、未確定な効果量を分ける。

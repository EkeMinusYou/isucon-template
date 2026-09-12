# Topology

役割の正本は`Taskfile.yml`冒頭のrole変数とIP mapである。RUNの測定値を配置へ帰属する際は`run.json`と当時の実効構成を照合する。現行構成が異なる場合やRUNがない場合も配置案を探索できるが、測定事実と現行構成からの仮説を分ける。

- app、nginx、DB、外部serviceの需要とcapacity
- cross-host往復、connection、serialization
- state ownerとtraffic入口
- 移設後の最大需要/capacityと障害時guardrail
- deploy、初期化、再起動後の収束

役割変更は複数service・hostにまたがっても、一体で採否・適用するなら一つのInterventionとする。

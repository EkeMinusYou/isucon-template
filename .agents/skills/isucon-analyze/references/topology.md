# Topology

役割の正本は`Taskfile.yml`冒頭のrole変数とIP mapである。対象RUNの`run.json`と一致する構成だけを原因Evidenceに使う。

- app、nginx、DB、外部serviceの需要とcapacity
- cross-host往復、connection、serialization
- state ownerとtraffic入口
- 移設後の最大需要/capacityと障害時guardrail
- deploy、初期化、再起動後の収束

役割変更は複数service・hostにまたがっても、一体で採否・rollbackするなら一つのInterventionとする。

# Known solutions

`docs/special-sources/`と`docs/solutions/`は候補源であり、現行への適用指示ではない。

## 共通の評価手順

`isucon-analyze` の既知資料探索と、`isucon-special-sauce` / `isucon-use-solution` の対象限定調査で共用する。対象の選び方は呼び出し元に従う。

1. `AGENTS.md`、[Backlog三層](../../_shared/objective-constraint-intervention.md)、[Evidence](../../_shared/evidence.md)、対象に関係する `docs/official/`、`tools/backlog/README.md` と `tools/backlog/backlog-workflow.md` を読む。
2. `git status --short` と必要な diff、Taskfile の役割・IP map、対象のコード・schema・設定を確認する。生成物は生成元を追い、Node.js 参考実装を変更対象にしない。
3. `task backlog -- objective list`、`task backlog -- constraint list --all`、`task backlog -- list --no-color` で ACTIVE Objective、Constraint、進行中 Intervention を確認する。
4. 資料の前提と現行の配置・version・データライフサイクル・公式 guardrail を照合する。現行ですでに成立していないか、部分適用なら未被覆の残差があるか調べる。
5. 対象処理・資源と削減機構、Objective へ効く方向を説明する。既存計測は任意の補助情報であり、効果量未知、主要ボトルネックでないこと、変更規模だけで候補を落とさない。RUN を使う場合は指定 RUN、指定なしなら finalized な最新 RUN を優先し、`run.json` の source・役割・artifact status を確認する。過去 RUN は比較目的を明示する。
6. 公式仕様違反、前提不一致、現行で解消済み、改善方向を説明不能、現在進行中の別カードによる完全被覆は起票せず具体的な理由を残す。被覆は target・mechanism・Change boundary と独立した残差の有無で判断する。`VALIDATED` / `REJECTED` の status や過去理由だけで棄却しない。

## Backlogへの引き渡し

書き込み直前に進行中カードを再読し、`task backlog` 経由で候補単位の Owner なし `INVESTIGATE` を作る。CLI の引数は現行 README を確認し、本文は `--section-stdin` で登録する。

- Observation: 元文書へのリポジトリ相対 Markdown リンク（節が分かればアンカー付き）、現行事実、適用前提、公式資料。計測を使った場合は RUN と成果物、単位・母数・snapshot。
- Hypothesis: Objective へ至る因果、期待する方向、未検証の効果。
- Change boundary: 一体で採否・適用・切り戻す対象、生成元を含む管理対象。
- Safety: 仕様 guardrail、停止条件、rollback 方針。
- Verification / Unknowns: 既存の標準 Evidence での判定方法と、後続調査で独立確認する事項。
- 候補の識別: target・mechanism・premise と Change boundary を本文に具体化する。Intervention の識別子はカード ID であり、別の Fingerprint は作らない。

少なくとも一つの ACTIVE Objective へ `objective link` で因果を登録する。成立条件を満たす Constraint がある場合だけ `RESOLVES` / `MITIGATES` を登録し、性能 Constraint の `RESOLVES` では structured residual assessment を満たす。Constraint を起票の必須前提にしない。

同じ進行中カードが被覆する場合は必要な出典・Evidence を補足し、状態や Owner は変えない。更新には直前の Card version、Objective・Constraint の更新にはそれぞれの version を使い、actor と reason を記録する。競合したら再読してマージする。書き込み後に `task backlog -- validate` を実行する。

READY 化と既存 Intervention の状態遷移は行わず、`isucon-investigate` へ引き渡す。情報不足・追加計測だけをカード化しない。

## 操作範囲

コード・設定・元資料・保存済み RUN の読み取りと必要最小限の読み取り専用 SSH に限る。実装、build、gen、deploy、サービス操作、bench、新規計測、成果物の再集計は行わない。書き込みは Backlog と必要な分析レポートのみ。レポートを保存する場合は `docs/reports/README.md` の命名・非上書き規則に従う。

文書を作ること自体をBacklog作業にしない。

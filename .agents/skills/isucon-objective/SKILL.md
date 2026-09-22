---
name: isucon-objective
description: 計測と公式仕様から得点への寄与仮説をObjectiveとして作成・再評価する。Target管理・実現方法の探索は行わない。
---

# ISUCON objective

有効な最終スコアの最大化を最上位目的とし、スコアへの寄与経路をObjectiveとして定義・管理する。Objectiveは、どの得点機会の成立・進行や損失の抑止を通じてスコアを増やすかを表す。待ち時間短縮や資源効率改善は複数経路に共通する改善手段であり、それだけをObjectiveの分類軸にしない。仕事や資源消費の増加を伴う方針もスコアへの純寄与で評価する。具体的なObjective・Targetの例示や固定分類で探索を誘導せず、初期整備と既存Objectiveの見直しを同じ入口で扱う。

## 最初に読む

- `AGENTS.md`、当日の採点・有効性・追試条件に関する`docs/official/`。
- [Backlog workflow](../../../tools/backlog/backlog-workflow.md)の`Authority`・`Objective`・`Common validity conditions`・`Relations`・`Evidence policy`・`Writer protocol`。
- [Backlog CLI](../../../tools/backlog/README.md)、[Evidence](../_shared/evidence.md)。
- 過去ID・移行判断を辿る必要がある場合は[Objective organization](../../../tools/backlog/objective-organization.md)。移行記録のIDや当時の状態を現行台帳の代用にしない。
- 寄与機構を調べる際は[score mechanics](../isucon-analyze/references/score-mechanics.md)、[benchmark behavior](../isucon-analyze/references/benchmark-behavior.md)の必要部分。

## 境界

Objectiveの作成・更新・退役とレポートを担当する。Targetの内容・状態・Objectiveリンクは[isucon-target](../isucon-target/SKILL.md)の担当とし、Interventionの起票・状態変更は行わない。保存済みEvidence、現行コード・設定、公式資料と必要最小限の読み取り専用SSHを使う。実装、設定変更、deploy、service操作、bench、保存済み成果物の再生成・上書き、当該競技やベンチマーカーのインターネット調査は行わない。

## 手順

1. `task backlog -- objective list`でACTIVE Objectiveを確認し、必要なIDを`objective show`で読む。`task backlog -- target list`と関連Intervention、Historyから影響範囲を把握する。退役Objective・終端Targetは重複や過去の判断に関係するIDを移行記録やHistoryから辿ってshowで確認する。過去のIntervention一覧が必要なら`task backlog -- list --all --target A-ID`で絞る。ユーザーが範囲を限定していなければ全ACTIVE Objectiveを再評価し、指定があればその範囲を扱う。
2. 過去RUNのベンチログとユーザー遷移メトリクスを主な観測根拠として、下記の得点源分析を行う。最新RUNだけでなく、寄与仮説の比較に必要な過去RUNを選ぶ。まず得点の発生とそれを支える経路を洗い出し、独立した寄与仮説・同じ経路の構成要素・未確定の経路を区別する。一つの経路を観測できたことを、それだけが得点源である根拠にしない。既存Objectiveや初期値を探索の固定分類にしない。
3. 既存Objectiveは[寄与仮説の再評価](../_shared/evidence.md#寄与仮説の再評価)に従い、従来仮説と改善後の観測を照合する。継続・重点変更・先に解く問いの判断と参照カードID・RUNを、対象ObjectiveのHistoryへ残す（検査のみなら更新案に含める）。維持する場合も観測を踏まえた理由を示し、状態変更自体は必須にしない。方針の存続と探索順を分け、[Work selection](../../../tools/backlog/backlog-workflow.md#work-selection)に沿って全対象の寄与経路、優先順と理由、未確認の問いをTarget担当へ渡す。重点指定によって他のACTIVE Targetを探索対象から外さない。観測が不足する関係は、何を確認すれば判断が変わるかまで具体化する。
4. 寄与経路の整理からObjectiveの維持・追加・修正・統合・分割・退役を判断する。独立してスコアへ寄与する仮説は別Objectiveとし、同じ寄与経路内のAPIや改善手段を機械的に分割しない。既存の広いObjectiveに異なる寄与経路をまとめて本文へ追記するだけで終えず、タイトル・定義・評価条件を経路に合わせて再構成する。既存Objectiveを狭める場合は、外れる経路を別Objectiveへ移すか、根拠不足・見送りとして残すかを判断し、黙って落とさない。暗黙の最上位目的、共通の有効性条件、明示要求された別賞の目的はworkflowに従って区別する。
5. 各方針に根拠RUN・観測事実・推定した寄与経路・成立条件・不確実性・評価可能な指標や述語・支持と反証の観測・見直し条件を記録する。得られる利益と追加費用から純寄与を判断し、局所指標の増減だけで得点効果を認定しない。寄与額の確定や個別変更の因果分離をObjective作成の前提にせず、観測に基づく推定を候補選択に使い、後続RUNで更新する。現時点の律速や即時の得点増加も必須条件にしない。
6. 廃止は履歴を保存した退役で表し、統合・分割では旧方針と新方針の関係と理由をHistoryへ残す。ACTIVE Targetが参照するObjectiveの退役は、isucon-targetで必要なリンク整理が済むまで保留し、対象IDと理由を報告する。Objectiveを改定した場合も既存Targetの適合性を確認し、必要な変更を引き渡す。関連エンティティの状態を連動変更しない。
7. 書込み直前にversionを読み、Writer protocolとCLIでObjectiveを更新する。actorは`skill:isucon-objective`を使う。履歴を保存し、`task backlog -- validate`で整合性を確認する。検査のみの依頼では更新案までに留める。

## 得点源の分析

各寄与経路について、何が増える・成立する・失われなくなると、どの得点へつながるのかを説明する。この説明が異なり、別々に支持・反証を評価できるものをObjectiveの境界とする。複数経路が同じ資源を使うことや、同じ高速化で改善することだけでは統合しない。APIやシナリオが異なることだけでも分割しない。タイトルはこの寄与経路と改善方向が分かる具体性を持たせ、本文と一緒に更新する。

ベンチログの得点・シナリオの進行と完了、ユーザー遷移メトリクスの観測経路と頻度・時間を突き合わせ、得点に寄与するシナリオやAPIとその関係を推定する。直接得点が発生する箇所だけでなく、その成立・進行を左右する経路も辿る。公式仕様は採点の意味と正当性条件を確認する資料、コード・設定は観測された関係の機構を確認する資料として使う。仕様から方針を先に固定してログを当てはめない。

数値集計・RUN横断比較は既存DuckDB基盤を優先する。`tools/analysis/sources.yaml`と関連schema、`tools/user-transition-metrics/README.md`を確認し、既存の取り込み定義と集計の意味を使う。計測基盤の変更や新たなベンチ実行を分析の前提にしない。

`run.json`で成果物のstatusとsource・構成・計測条件を確認し、実負荷と集計の時間窓、母数、欠損、RUN間の変更を区別する。遷移のシナリオ群は観測API集合による分類であり、ベンチのシナリオ名と同一視しない。対応付けの根拠と曖昧さを残す。識別情報のない要求、順序が曖昧な遷移、重なった要求、集約によって失われた情報を考慮し、観測上の隣接や相関を因果の証明にしない。群別・API別の得点額が直接得られなければ捏造せず、取得できる観測と機構から寄与を推定する。

推定の不確実性だけで起票を止めず、根拠のある仮説と裏付けのない推測を区別する。比較条件や観測範囲が限られる場合は、その条件内で言えることを明記する。過去RUNがない初期整備や資料欠損時は、利用可能な公式仕様・コード等から暫定仮説を作り、未観測の前提と後続RUNで確認する条件を残す。

## 完了と引き継ぎ

再評価では、従来の寄与仮説について今回支持・反証された関係と、未確認で残る関係を示す。Target側で前提や評価条件の見直しが必要な場合は、対象IDと根拠を引き渡す。

分析したRUNと成果物、得点源・寄与経路の推定と根拠、経路とObjectiveの対応、旧Objectiveからの再構成判断、変更前後のタイトルと更新ID、未確定点と見直し条件を報告する。維持する場合も寄与経路として適切な粒度である理由を示し、局所的な高速化の観測だけを維持理由にしない。Target側の整理が必要なら対象Objective／Target ID、理由、参照Evidenceを示し、isucon-targetへの引き継ぎ事項として残す。新スキルを自動起動して作業範囲を広げない。

RUNを使った報告は[レポート命名規則](../../../docs/reports/README.md)に従い`docs/reports/isucon-objective/`へ新規保存する。RUNがない初期整備はCLIのHistoryと完了報告に残し、架空のRUNを作らない。

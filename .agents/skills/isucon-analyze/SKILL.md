---
name: isucon-analyze
description: 指定なしなら全ACTIVE Targetの全観点で残存する仕事を確認し、利用可能なEvidenceから独立した改善候補を探索してINVESTIGATEへ起票する。1件で探索を終了せず、READY判定・実装は行わない。
---

# ISUCON analyze

既存ACTIVE Targetの目標を実現するInterventionを探索する。現在の律速だけでなく、追加高速化や将来の負荷条件も扱う。Target改善からObjectiveを通じて得点へ効く機構と前提を説明し、局所改善と得点効果の確認を区別する。起票時にChange boundaryを完成させることは求めず、空または暫定の範囲をinvestigateへ渡してよい。

指定なしなら全ACTIVE Targetを詳細探索し、ユーザーが明示的に限定した場合だけ対象を絞る。Priority・Evidence・前工程の推薦は調査順と各Target内の問いに使い、対象の除外理由にしない。未達の観測と達成判定に必要なEvidenceの不足を区別する。現行実装・保存済みRUN・現在のopen Interventionと各Targetの直近判断を現在の出発点として扱い、対象全件の成果は[終了条件](#調査を続ける条件と終了)で確認する。

## 最上位目的と必須完了ゲート

最上位目的は、公式の正当性を維持した最終スコアの最大化である。有望な改善案を1件見つけても、それだけで調査を終了しない。各ACTIVE Targetについて、後述の全観点を確認し、残存する仕事を新規起票・既存openカード修正・同一変更・見送り・未解決のいずれかに分類する。

候補数にかかわらず、各観点について残存仕事・Evidence・判断・再開条件を記録する。「既存openカードがある」「優先度が低い」「効果未計測」「最新RUNが成功」は、単独では探索や起票を省略する理由にならない。効果量が未確定でも、処理削減からTarget指標・得点または損失回避へ至る因果仮説を説明できればINVESTIGATEへ起票する。

## 読む資料と作業境界

- `AGENTS.md`、対象に関係する`docs/official/`
- [Backlog workflow](../../../tools/backlog/backlog-workflow.md)の`Authority`・`Objective`・`Common validity conditions`・`Target`・`Intervention`直下・`Relations`・`Evidence policy`・`Priority`・`Writer protocol`。指定節を検索して読み、読了済みなら再読しない。
- [Evidence](../_shared/evidence.md)。CLIで書き込む際は[Backlog README](../../../tools/backlog/README.md)と必要なコマンドのhelpを確認する。

担当範囲に必要なreferenceだけ読む: [performance](references/performance.md)、[score mechanics](references/score-mechanics.md)、[benchmark behavior](references/benchmark-behavior.md)、[topology](references/topology.md)、[known solutions](references/known-solutions.md)。

公式資料、現行コード・設定、保存済みEvidenceと必要な読み取り専用SSHを使う。保存済みデータへの読み取り集計は可能で、既存DuckDB基盤を優先する。RUN成果物の再生成・上書き、実装・設定変更、deploy、service操作、bench、当該競技とベンチマーカーのインターネット調査は禁止する。

Backlogへの書込みは親だけが行い、子は読み取り専用の調査と親への報告に限る。ObjectiveとTargetの定義・状態・TargetからObjectiveへのリンクは変更しない。例外として、Targetの目標達成を示す事実は既存Evidenceを保持してTargetカードへ追記し、達成判定と状態変更は[isucon-target](../isucon-target/SKILL.md)へ任せる。適切なTargetがない候補や目標・前提の問題は根拠付きで報告し、Targetの新設・拡張や別スキルの自動起動は行わない。

## 入力確認とTargetごとの調査判断

親が`task backlog -- objective list`、`task backlog -- target list`、open Interventionと直近の探索記録を確認する。関連するopenカードはIDのshowを使い、一覧が必要なら`task backlog -- list --target A-ID`のように対象を絞る（`--all`は使わない）。

ユーザー指定RUN、指定なしなら最新finalized RUNの`run.json`でsource・役割・APPLIED snapshot・artifact statusを確認し、現行コード・設定との差を整理する。RUNがなくても公式資料とコード等で機構を説明できれば探索・起票する。負荷時の効果は未確定とし、架空のRUNや数値は作らない。

[寄与仮説の再評価](../_shared/evidence.md#寄与仮説の再評価)に従って関連するACTIVE Targetとopen InterventionのHistory・観測を照合し、[Work selection](../../../tools/backlog/backlog-workflow.md#work-selection)に沿って対象全件の調査順と各Target内の問いを選ぶ。前工程の推薦があればその根拠を引き継ぎ、前提が変わった部分を再評価する。同じ未確認の寄与関係に依存する追加案は、追加投資を支持する根拠と参照するopenカードID・RUNをHypothesisへ残し、更新時の判断理由をHistoryへ記録する。

概観では各Targetの直近の目標評価・観測・寄与判断を再利用する。各Targetについて目標・評価条件を対象RUNの観測と照合し、未達を観測した場合と、必要なEvidenceがなく確認できない場合を区別する。数値集計は既存DuckDB基盤を優先し、観測結果とコード・実効設定から、残る仕事や待ちと次に調べる問いを選ぶ。確認できない条件を一括して後続担当の検証事項にせず、選択判断を変え得る部分を保存済みEvidenceや読み取りSSHで確認する。

概観を調査順の判断に使った後、対象全件について次の項目を確認・記録する。既存の詳細調査は、参照先・snapshot・調べた問いを示し、現行との差分と再探索条件を照合して再利用できる。概観や優先度の説明だけを詳細探索の代わりにしない。

| 項目 | 判断内容 |
| --- | --- |
| 目標と前提 | 対象・目標・評価条件、ACTIVE Objectiveへの寄与仮説、baselineと現行状態 |
| 分析対象のIntervention | ID・状態・Ownerと、各案が扱う機構・変更境界（確定・暫定・未記載を区別）。存在しなければ「なし」 |
| 最新の観測 | 対象RUNで目標・評価条件について確認できた事実と確認できない条件 |
| 既存変更の効果と限界 | 削った仕事と残る処理・待ち・競合、成立条件。観測した効果と見込みを区別する |
| 今回の探索 | 残る仕事の削減、補完策、既存方式の代替策を調べる範囲と具体的な問い |
| 未解決の問い | 未確認事項、次に確認するEvidence、調査終了時には利用可能な手段で解けない理由と再開条件 |

分析時に確認するInterventionはINVESTIGATE／READY／DOING／VERIFY／APPLIED／BLOCKEDを指す。VALIDATED／REJECTEDは終端記録であり、重複確認や過去調査の参照対象にしない。過去の比較が必要な場合は、現行実装・保存済みRUN・openカードのHistoryから確認する。

同じ処理経路・機構・Targetに触れるだけでは同一変更ではない。同一変更は、同じTargetに対する同じ機構について、対象範囲・変更内容・応答や正当性の成立条件を含むChange boundaryが実質同一の候補を指す。残る仕事、成立条件、変更箇所のいずれかが異なる補完策・代替策は、同じ機構でも別候補として比較・起票する。既存案との比較には機構・成立条件・適用見通しを使い、厳密な削減量を探索・起票の前提にしない。

対応するACTIVE Objectiveや有効な目標・評価条件がないTargetでは起票せず、問題を根拠付きで担当へ報告する。Targetの定義と観測が食い違う場合も、analyze側で目標を拡張したり未達の事実を作ったりせず、定義の範囲内で調べられる問いを進める。

## 分担と探索の進め方

対象全件を、同じEvidenceで判断できるコード・データ・処理経路の共有関係でクラスタ化する。件数の均等化や同じObjectiveへの所属だけでまとめない。局所改善と構造変更の比較は[共通の評価基準](../_shared/evidence.md#改善方向と次の投資判断)に従う。前回と入力・問いが同じ部分は根拠を再利用し、新しい問いや前提の変化を詳しく調べる。

クラスタごとにサブエージェントへTarget ID、固定入力となる目標・評価条件・Objectiveへの因果、共通snapshot、既存open案、今回の問いと担当範囲を渡す。各Targetの主担当は一つにし、横断Targetの調査と各API調査の範囲の重なりを親が調整する。利用枠が足りなければ順次委任するか親が担当する。

各担当は追加改善を探索し、[終了条件](#調査を続ける条件と終了)に沿って親へ結果を返す。

各担当は開始時に具体的な探索範囲と選択理由を親へ報告する。範囲数に下限・上限は設けない。新しい発見から必要な範囲が増えたら親へ共有し、分担を調整して続ける。共有するSSH確認は親がまとめて行い結果を渡すが、新しい問いに必要な追加確認は行う。

| 観点 | 探索対象 |
| --- | --- |
| DB・データ処理 | SQL、索引、走査、集計、データ増加時の仕事量 |
| 要求・シナリオ待ち | 直列処理、重複取得・計算、転送、次操作への進行 |
| 並行処理・競合 | ロック、Tx、接続保持、共有状態 |
| 得点・損失 | 得点が発生する操作までの進行、失敗・待ちによる損失 |
| 配置・容量 | CPU・I/O・通信、ホスト間の余力、役割配置 |

観点は担当を分ける単位ではない。各ACTIVE Targetで全観点を確認し、該当しない場合も「該当なし」とEvidence付きで記録する。詳細な調査深度はEvidenceと問いに応じて決め、全指標・経路の再集計を意味しない。現在の影響と、将来どの負荷条件で効くかを確認する。

## 候補の報告とBacklogへの反映

親は新規起票・編集可能なOwnerなしINVESTIGATEの更新時に、[workflowのPriority](../../../tools/backlog/backlog-workflow.md#priority)に従って優先度を設定・見直してよい。未設定もこの機会に埋める。

候補ごとに以下が揃った時点で親へ報告し、他クラスタの完了を待たず反映する。

- Observation: Evidenceとsnapshot、確認した処理機構。数値を示す場合は単位・母数を付ける
- TargetとHypothesis: 既存ACTIVE Targetへの寄与、Objectiveから得点へ効く因果と成立条件
- Change boundary: 分かっている対象範囲・除外範囲、既存案との差分・同一変更かどうか・競合。起票時に未確定なら空または暫定でよく、調査で決める問いをUnknownsへ残す
- Evaluation: 構造削減、応答意味と共通の有効性条件、負荷時効果の評価計画。コード・SQL構造の検証、同一入力での応答比較、適用後の保存RUNや読み取りSSH等による採用・修正・不採用基準
- 未確定点と、詳細検証で確認する内容

候補の詳細方式と最終的なChange boundaryはINVESTIGATE後に具体化する。評価計画と評価実行を区別し、情報不足や追加計測だけをカード化しない。

親は直前にTarget・Objectiveと関連するopen Interventionの状態・Owner・前提を再読し、次の規則で処理する。

| 既存案との関係 | 処理 |
| --- | --- |
| 既存案と異なる追加の問い・変更内容を持つ補完策・代替策（同じ機構でも可） | OwnerなしINVESTIGATEを新規作成。既存ACTIVE Targetへ作成時にリンクし、主Targetは一つにする。Change boundaryが暫定・未記載でも、仮説とEvidence、調査で具体化する問いを記録する |
| 既存案と同じ提案の修正・詳細化で、既存openカードがINVESTIGATEかつOwnerなし | 既存openカードを修正し、理由と従来案からの変更をHistoryへ残す。状態とOwnerは維持する |
| 既存案と同じ提案の修正で、上記の編集条件を満たさない | 対象ID・根拠・修正案を完了報告または親への引き継ぎに記録してその案の調査を終える。既存openカードは変更せず、同一変更のカードも作らない |
| 同一のChange boundaryが既にopen Interventionとして起票済み、または現行実装で成立済み | 同一変更として新規起票せず根拠を記録する。ただし、そこから残る別の仕事は別候補として扱う |

Targetの達成を示すEvidenceを見つけた場合は、既存Evidenceを保持してTargetカードへ追記し、達成判定・状態変更は`isucon-target`へ渡す。Target Evidenceの追記とBacklog書込みは、[Backlog workflowのWriter protocol](../../../tools/backlog/backlog-workflow.md#writer-protocol)に従う。analyzeはTargetの定義・状態・Objectiveリンクを変更しない。競合やOwner付きのopenカードは再読して意図的に統合し、上書きしない。

## 調査を続ける条件と終了

終了前に対象全件の必須完了ゲートを検査する。ACTIVE Targetやopen Interventionの既存調査を再利用する場合も、既存Evidence・現行との差分・未探索の問い・再開条件を確認する。既存実装の確認、候補1件の発見、同一変更の判定、低優先度や既存InterventionのOwnerだけでは終了できない。不足があれば追加調査へ戻す。

選んだ寄与仮説について採否・優先順位・変更境界を変え得る具体的な問いがあり、リポジトリ、保存済みEvidence、読み取りSSHで進められる間は調査を続ける。まだ読んでいない資料・コード・実効設定を「根拠不足」として終了せず、RUN・コードに差分がないことだけで未探索部分を除外しない。調査済みの同一前提を繰り返さず、新しい問いや成立した再探索条件へ進む。新RUNや新着カードなどで前提が変わったら、親が対象全件への影響を確認し、調査順と問いを更新する。

各Targetで全観点の判断と、利用可能な手段で進められる問いの処理が揃えば調査を終了する。候補数・範囲数を終了条件にせず、全観点を根拠付きで分類した場合に限り候補ゼロも認める。未確定の案は、確認したEvidence、利用可能な手段では解けない理由、再開に必要な実装・ユーザー実行のベンチ・外部状態の変化を残す。その案を進められない場合や既存openカードを編集できない場合も、他の独立した問いは継続する。

## 調査記録と完了報告

調査結果はBacklogカードのObservation・Hypothesis・Evaluation・Historyと完了報告へ記録する。`isucon-analyze`固有に、Targetごとの全観点の判断、候補の扱い、未解決点、終了理由、再開条件を記録する。全観点の判断はカードまたは完了報告の簡潔な一覧で示し、候補数や網羅的な比較表を要件にしない。

全担当の結果を回収し、終了直前にACTIVE Target一覧を再読してユーザー指定範囲と照合する。不足があれば調査へ戻り、`task backlog -- validate`を通す。後続担当へは今回進める寄与仮説と対応IDを根拠・snapshotとともに渡し、起票した全件を自動的な着手対象にしない。

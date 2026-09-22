---
name: isucon-analyze
description: 既存ACTIVE Targetを実現する改善方法を探索し、INVESTIGATEへ起票する。READY判定・実装は行わない。
---

# ISUCON analyze

既存ACTIVE Targetの目標を実現するInterventionを探索する。現在の律速だけでなく、追加高速化や将来の負荷条件も扱う。Target改善からObjectiveを通じて得点へ効く機構と前提を説明し、局所改善と得点効果の確認を区別する。

ユーザーによる範囲指定がなければ全ACTIVE Targetを対象とし、各Targetの達成根拠を確認して追加改善を探索する。優先度は探索順に使い、対象から除外する理由にしない。未達と達成判定に必要なEvidenceの不足を区別する。達成根拠が揃った場合はisucon-targetへRESOLVED判定を引き継ぎ、analyze自身は状態を変更しない。既存実装・採用済みInterventionは比較の出発点とし、対象ごとの成果を[終了条件](#調査を続ける条件と終了)で確認する。

## 読む資料と作業境界

- `AGENTS.md`、`docs/reports/README.md`、対象に関係する`docs/official/`
- [Backlog workflow](../../../tools/backlog/backlog-workflow.md)の`Authority`・`Objective`・`Common validity conditions`・`Target`・`Intervention`直下・`Relations`・`Evidence policy`・`Priority`・`Writer protocol`。指定節を検索して読み、読了済みなら再読しない。
- [Evidence](../_shared/evidence.md)。CLIで書き込む際は[Backlog README](../../../tools/backlog/README.md)と必要なコマンドのhelpを確認する。

担当範囲に必要なreferenceだけ読む: [performance](references/performance.md)、[score mechanics](references/score-mechanics.md)、[benchmark behavior](references/benchmark-behavior.md)、[topology](references/topology.md)、[known solutions](references/known-solutions.md)。

公式資料、現行コード・設定、保存済みEvidenceと必要な読み取り専用SSHを使う。保存済みデータへの読み取り集計は可能で、既存DuckDB基盤を優先する。RUN成果物の再生成・上書き、実装・設定変更、deploy、service操作、bench、当該競技とベンチマーカーのインターネット調査は禁止する。

レポートとBacklogの書込みは親だけが行い、子は読み取り専用の調査と報告に限る。ObjectiveとTargetの定義・状態・TargetからObjectiveへのリンクは変更しない。例外として、Targetの目標達成を示す事実は既存Evidenceを保持してTargetカードへ追記し、達成判定と状態変更は[isucon-target](../isucon-target/SKILL.md)へ任せる。適切なTargetがない候補や目標・前提の問題は根拠付きで報告し、Targetの新設・拡張や別スキルの自動起動は行わない。

## 入力確認とTargetごとの調査判断

親が`task backlog -- objective list`、`task backlog -- target list`、open Interventionと直近の探索記録を確認する。過去カードは関連IDのshowを使い、一覧が必要なら`task backlog -- list --all --target A-ID`のように対象を絞る。

ユーザー指定RUN、指定なしなら最新finalized RUNの`run.json`でsource・役割・APPLIED snapshot・artifact statusを確認し、現行コード・設定との差を整理する。RUNがなくても公式資料とコード等で機構を説明できれば探索・起票する。負荷時の効果は未確定とし、架空のRUNや数値は作らない。

[寄与仮説の再評価](../_shared/evidence.md#寄与仮説の再評価)に従って関連Historyと観測を照合し、[Work selection](../../../tools/backlog/backlog-workflow.md#work-selection)に沿って対象ごとの問いと探索順を決める。呼び出し側の重点指定は優先順として引き継ぎ、ユーザーが範囲を限定していない限り他のACTIVE Targetも探索する。同じ未確認の寄与関係に依存する追加案は、追加投資を支持する根拠と参照カードID・RUNをHypothesisへ残し、更新時の判断理由をHistoryへ記録する。

概観では各Targetの直近の目標評価・観測・寄与判断を再利用する。対象の全Targetについて目標・評価条件を対象RUNの観測と照合し、未達を観測した場合と、必要なEvidenceがなく確認できない場合を区別する。数値集計は既存DuckDB基盤を優先し、観測結果とコード・実効設定から、残る仕事や待ちと次に調べる問いを選ぶ。確認できない条件を一括して後続担当の検証事項にせず、選択判断を変え得る部分を保存済みEvidenceや読み取りSSHで確認する。

指定なしなら全ACTIVE Target、ユーザーが範囲を限定した場合はその範囲について、次の項目を確認・記録する。概観や過去レポートの見送り理由の参照だけで探索済みとしない。既に同じ問いを十分に調査済みなら結果を再利用できるが、現行への適合と残る独立した問いを確認する。「新Evidenceがない」「低優先度」「他Targetを優先する」だけで未探索のTargetを対象外にしない。

| 項目 | 判断内容 |
| --- | --- |
| 目標と前提 | 対象・目標・評価条件、ACTIVE Objectiveへの寄与仮説、baselineと現行状態 |
| アクティブなIntervention | ID・状態・Ownerと、各案が扱う機構・変更境界。存在しなければ「なし」 |
| 最新の観測 | 対象RUNで目標・評価条件について確認できた事実と確認できない条件 |
| 既存変更の効果と限界 | 削った仕事と残る処理・待ち・競合、成立条件。観測した効果と見込みを区別する |
| 今回の探索 | 残る仕事の削減、補完策、既存方式の代替策を調べる範囲と具体的な問い |
| 未解決の問い | 未確認事項、次に確認するEvidence、調査終了時には利用可能な手段で解けない理由と再開条件 |

アクティブなInterventionはINVESTIGATE／READY／DOING／VERIFY／APPLIED／BLOCKEDを指す。VALIDATED／REJECTEDは過去の実装・判断を知るEvidenceであり、現在の作業を被覆しているとは数えない。過去の採否だけで候補を棄却せず、現行前提と再探索条件を確認する。

同じ処理経路や変更境界も代替案の探索対象になる。既存案との比較には機構・成立条件・適用見通しを使い、厳密な削減量を探索・起票の前提にしない。

対応するACTIVE Objectiveや有効な目標・評価条件がないTargetでは起票せず、問題を根拠付きで担当へ報告する。Targetの定義と観測が食い違う場合も、analyze側で目標を拡張したり未達の事実を作ったりせず、定義の範囲内で調べられる問いを進める。

## 分担と探索の進め方

対象の全Targetを、同じEvidenceで判断できるコード・データ・処理経路の共有関係でクラスタ化する。件数の均等化や同じObjectiveへの所属だけでまとめない。局所改善と構造変更の比較は[共通の評価基準](../_shared/evidence.md#改善方向と次の投資判断)に従う。前回と入力・問いが同じ部分は根拠を再利用し、新しい問いや前提の変化を詳しく調べる。

クラスタごとにサブエージェントへTarget ID、固定入力となる目標・評価条件・Objectiveへの因果、共通snapshot、既存案、今回の問いと担当範囲を渡す。対象の全Targetに主担当を一つずつ割り当て、横断Targetの調査と各API調査の重複を親が調整する。利用枠が足りなければ順次委任するか親が担当する。

各担当は追加改善を探索し、[終了条件](#調査を続ける条件と終了)に沿って親へ結果を返す。

各担当は開始時に具体的な探索範囲と選択理由を親へ報告する。範囲数に下限・上限は設けない。新しい発見から必要な範囲が増えたら親へ共有し、分担を調整して続ける。共有するSSH確認は親がまとめて行い結果を渡すが、新しい問いに必要な追加確認は行う。

| 観点 | 探索対象 |
| --- | --- |
| DB・データ処理 | SQL、索引、走査、集計、データ増加時の仕事量 |
| 要求・シナリオ待ち | 直列処理、重複取得・計算、転送、次操作への進行 |
| 並行処理・競合 | ロック、Tx、接続保持、共有状態 |
| 得点・損失 | 得点が発生する操作までの進行、失敗・待ちによる損失 |
| 配置・容量 | CPU・I/O・通信、ホスト間の余力、役割配置 |

観点は担当を分ける単位ではなく、各クラスタで必要に応じて使う。現在の影響と、将来どの負荷条件で効くかを確認する。

## 候補の報告とBacklogへの反映

親は新規起票・編集可能なOwnerなしINVESTIGATEの更新時に、[workflowのPriority](../../../tools/backlog/backlog-workflow.md#priority)に従って優先度を設定・見直してよい。未設定もこの機会に埋める。

候補ごとに以下が揃った時点で親へ報告し、他クラスタの完了を待たず反映する。

- Observation: Evidenceとsnapshot、確認した処理機構。数値を示す場合は単位・母数を付ける
- TargetとHypothesis: 既存ACTIVE Targetへの寄与、Objectiveから得点へ効く因果と成立条件。Target全体の改善余地と、この候補が実際に改善する処理・待ち・損失の範囲を分け、現在の得点へ効く条件と支持するEvidenceを示す。将来の負荷条件で効く場合は区別し、Target全体の重要性を候補の効果として扱わない。効果量未確定でも起票できる
- Change boundary: 一体で採否・適用する境界案、既存案との独立性・重複・競合
- Evaluation: 構造削減、応答意味と共通の有効性条件、負荷時効果の評価計画。コード・SQL構造の検証、同一入力での応答比較、適用後の保存RUNや読み取りSSH等による採用・修正・不採用基準
- 未確定点と、詳細検証で確認する内容

機構と改善方向・因果を説明できれば、正確な削減量や時間帰属が未確定でもINVESTIGATEへ渡す。評価計画の記述と評価の実行は区別し、詳細方式の確定・検証とREADY判定はinvestigate以降の担当へ任せる。情報不足や追加計測そのものをカード化しない。

親は直前にTarget・Objectiveと関連Interventionの状態・Owner・前提を再読し、次の規則で処理する。

| 既存案との関係 | 処理 |
| --- | --- |
| 独立して採否・適用できる補完策・代替策 | OwnerなしINVESTIGATEを新規作成。既存ACTIVE Targetへ作成時にリンクし、主Targetは一つにする。代替案の排他性や競合も記録する |
| 既存案から独立できない修正・詳細化で、既存カードがINVESTIGATEかつOwnerなし | 既存カードを修正し、理由と従来案からの変更をHistoryへ残す。状態とOwnerは維持する |
| 既存案から独立できない修正で、上記の編集条件を満たさない | 対象ID・根拠・修正案をレポートへ記録してその案の調査を終える。既存カードは変更せず、重複カードも作らない |
| 同一案が既に起票済み、または現行実装で成立済み | 重複起票せず根拠を記録する |

Targetの達成を示す根拠が見つかった場合は、確認した事実・Evidence・snapshot・未確定点をTargetカードのEvidenceへ追記する。CLIの`target update --evidence`では直前に読んだ既存Evidenceを保持した全文に追記分を加え、`--expect-target-version`・actor・reasonを指定する。目標・評価条件・状態・Objectiveリンクは変更しない。同じ事実が記録済みなら重ねて追記せず、担当への引き継ぎに既存記録を参照する。

全書込みはWriter protocolとCLIに従い、対応するversion確認・actor・reasonを使う。競合したら再読して意図的に統合し、Ownerが付いた既存カード等へ上書きしない。

## 調査を続ける条件と終了

親は対象の全Targetについて、達成根拠、最新の観測、既存変更が削った仕事・残した仕事、実際に調べた補完策・代替策、未解決の問いを検査する。既存実装の確認、Evidence追記、局所案の発見や重複確認だけで対象全体の探索を終えない。不足があれば追加調査へ戻す。達成根拠が揃ったTargetは根拠をisucon-targetへ渡す。判定待ちを理由に同じGoalの追加案を無理に作らず、他のTargetを続ける。単独analyzeは状態変更や別スキルの自動起動を行わない。

各Targetの寄与仮説について採否・優先順位・変更境界を変え得る具体的な問いがあり、リポジトリ、保存済みEvidence、読み取りSSHで進められる間は調査を続ける。まだ読んでいない資料・コード・実効設定を「根拠不足」として終了せず、RUN・コードに差分がないことだけで未探索部分を除外しない。調査済みの同一前提を繰り返さず、新しい問いや成立した再探索条件へ進む。発見が前提を変えたら、親が各Targetの問いと探索順を更新する。

各案を新規起票・既存カード修正・修正案の報告・重複・見送り・未確定へ整理し、対象の全Targetで上記の問いを評価し終えたら今回の調査を終了する。未確定の案は、確認したEvidence、利用可能な手段では解けない理由、再開に必要な実装・ユーザー実行のベンチ・外部状態の変化を残す。その案を進められない場合や既存カードを編集できない場合も、他の独立した問いは継続する。候補数・範囲数を終了条件にせず、候補ゼロも認める。

## 探索記録と完了報告

レポートには解析日時、対象Objective・Target、RUNとコード・設定の時点、詳細探索したTargetごとの最新の観測・既存作業と残る仕事・調べた補完策と代替策・未解決の問い、クラスタと担当・探索範囲、事実・推論・未確定点、更新IDと修正せず報告した案を残す。終了理由、未探索部分と今回扱わなかった理由、再探索条件も記録する。レポート命名規則に従って新規保存する。RUNがない場合は専用のno-run命名を使い、編集禁止のカードに記録目的の書込みを行わない。

全担当の結果を回収し、対象の全Targetに達成根拠の確認と探索結果、または具体的な阻害条件があることを確認し、`task backlog -- validate`を通す。完了報告では選択理由、レポート、新規・修正Intervention、Targetへの追記、未確定点、終了理由と再開条件を示す。後続担当へは今回進める寄与仮説と対応IDを根拠・snapshotとともに渡し、起票した全件を自動的な着手対象にしない。

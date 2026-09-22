---
name: isucon-target
description: 既存Objectiveと保存済み計測から、改善対象・結果目標をTargetとして作成・再評価する。実現方法は探索しない。
---

# ISUCON target

既存Objectiveに対して「何を、どの指標で、どの条件で、どこまで改善するか」を管理する。計測結果から改善対象を選び、実現方法の探索は[isucon-analyze](../isucon-analyze/SKILL.md)へ渡す。

## WhatとHow

Objectiveは得点への寄与方針、TargetのWhatは改善対象と評価軸、Interventionは実現手段である。TargetカードではWhatをタイトル・Scope・Axisで特定し、その対象に対する目標をGoalに分けて記録する。具体性で区別しない。「特定APIのレイテンシ」は具体的なWhatであり、実装方式を指定していなくても「DB往復をなくす」「同じ値を再計算しない」はHowになり得る。

タイトルは対象と評価軸を表す名詞句にする。「改善する」「減らす」「短縮する」などの行動・変化の方向はタイトルに含めず、Goalに記載する。例えばタイトルは「ユーザー統計APIのレイテンシ」、Goalは「同等の負荷条件でbaselineより平均応答時間を短縮する」とする。タイトルのWhat、Goalの望ましい結果、InterventionのHowを区別し、タイトルを名詞句にしただけでGoalに残る実現手段の指定を見逃さない。

ScopeとAxisはAPI・シナリオ・共有処理・資源などの対象と、そのレイテンシ・CPU需要・処理容量・損失などの結果を特定する。SQL回数、コピー回数、cache hit等の機構指標は原因仮説・Intervention評価のEvidenceとして扱い、その削減をTargetのGoalに置かない。関数名や配置は計測箇所の説明であり、目標を現実装に固定する境界にしない。

別の実現手段でも同じ対象の結果が改善したなら達成できる目標かを確認する。同じAPIのレイテンシが改善しても「DB往復を減らしていないから未達」となるGoalは改定する。

## 最初に読む

- `AGENTS.md`と対象に関係する`docs/official/`。
- [Backlog workflow](../../../tools/backlog/backlog-workflow.md)のAuthority・Objective・Common validity conditions・Target・Relations・Evidence policy・Priority・Writer protocol。
- [Backlog CLI](../../../tools/backlog/README.md)、[Evidence](../_shared/evidence.md)、[レポート命名規則](../../../docs/reports/README.md)。
- 必要に応じて[score mechanics](../isucon-analyze/references/score-mechanics.md)、[benchmark behavior](../isucon-analyze/references/benchmark-behavior.md)。

## 担当範囲と参照資料

Targetの作成・更新・統合・退役・達成判定とTargetからObjectiveへのリンクを担当する。統合等に必要なInterventionからTargetへのリンク整理も扱うが、他Ownerのリンク変更は調整事項に残す。Interventionの起票・本文・状態・Owner・変更境界と、Objectiveの内容・状態は変更しない。

参照するのは公式仕様、Objective/Target/InterventionとHistory、保存済みRUN・ログ・profile・ユーザー遷移・集計値、および他スキルの調査・実装・検証報告である。アプリケーションのコード・設定を直接読んで原因や実現方法を分析せず、SSHで実効環境を調査しない。構成・適用時点はrun.jsonと担当報告で確認する。報告にコードの説明が含まれていても、削減案をそのままTargetへ昇格させない。

保存済みEvidenceへの読み取り集計・RUN比較は可能で、既存DuckDB基盤を優先する。その利用に必要なCLI資料・取り込み定義・schemaは読んでよい。計測基盤の変更、実装、設定変更、deploy、service操作、bench、保存済みRUN成果物の再生成・上書き、当該競技やベンチマーカーのインターネット調査は行わない。

原因・実現可能性・正当性の判断が不足する場合は、必要な問いと根拠をanalyzeまたは実装・検証担当へ返す。target自身がコード調査で埋めない。別スキルは自動起動しない。

## 現状確認と計測からの対象選定

`task backlog -- objective list`、`task backlog -- target list`でACTIVE一覧を確認し、対象IDをshowして関連InterventionとHistoryを読む。ユーザーによる範囲指定がなければ全ACTIVE Objectiveと全ACTIVE Target、指定があればその範囲を扱う。ACTIVE Objectiveがなければ不足を報告し、受け皿を作らない。

終端Targetは過去の判断や再発確認に必要なIDを辿る。過去Interventionは必要に応じて `task backlog -- list --all --target A-ID` で絞る。過去の達成は今回の改善余地がないという証拠にはしない。

指定RUN、指定なしなら最新finalized RUNのrun.jsonでsource・役割・APPLIED snapshot・artifact status・比較条件を確認する。担当報告から適用時点と未適用変更を区別する。RUNや評価指標が欠損している場合は捏造せず不足を記録する。公式・ユーザー要求から目標を定義できる場合も、計測済みbaselineとは区別する。

個々の実装案を追う前に、公式採点上の成果とObjectiveに対応する待ち・仕事量・容量・損失を概観する。既存カード順や前回の優先度を探索順にしない。優先仮説にはスコアへの因果、支持する計測、反証・代替仮説、不確実性を示す。要求数と一要求の費用、累積応答時間とCPU仕事、共有処理と個別APIを区別する。順位や割合だけで最大律速・最大得点寄与を断定しない。比較根拠が足りなければ有望な複数経路と不足情報を示す。

対象選定では[寄与仮説の再評価](../_shared/evidence.md#寄与仮説の再評価)に従い、関連History・担当報告と保存済み計測を照合する。TargetのGoal達成と、Objectiveへの寄与仮説の支持・反証・未確認を別々に判断する。

照合結果から、現行のScope・Goal・評価条件・Objectiveリンク・Priorityを維持または見直す理由を、参照カードID・RUNとともに対象TargetのHistoryへ残す（検査のみなら更新案に含める）。局所結果だけが改善した場合も、同じ優先度を維持する根拠を再評価する。Goal達成に得点増加を後付けで必須化せず、スコア横ばいだけでTargetを退役させない。

[Work selection](../../../tools/backlog/backlog-workflow.md#work-selection)に従い、対象の全ACTIVE Targetを達成済み・未達・判定材料不足に分ける。現Goalの達成根拠が揃ったTargetは下記の更新規則に従ってRESOLVEDへ移し、維持するTargetはWhatと判断を変える問いをanalyzeへ渡す。優先度は探索順として伝え、低優先度やEvidence不足だけで引き継ぎ対象から外さない。実現方法の選定やコード調査はanalyzeの担当とする。

Objective自体の仮説に問題がある場合は、対象ID・観測・未確認の関係をobjective担当へ返す。原因や実現方法の調査が必要な場合は、具体的な問いを担当へ引き渡す。

主要な経路について既存TargetのScope/Goalと照合し、「既存で被覆／新規Target／見送り／未確定」に分類する。未被覆部分は計測結果・遷移・担当報告から検討し、コードから追加の無駄を探す活動には広げない。現在の影響と将来の負荷条件を区別し、同じ対象の追加改善も評価する。

## Targetの定義・検査

- タイトルが対象・評価軸の名詞句になっているか、変化の方向・達成条件がGoalに分離されているかを確認する。対象、結果の評価軸、baseline、Goal、評価条件を対応させる。数値には単位・母数・時間窓・負荷条件を付ける。profileの関数消失や処理の移設を需要0と扱わない。
- 閾値に根拠がなければ仮の削減率を置かず、比較可能なbaselineに対する結果の改善をGoalにする。その場合も主指標・方向・比較条件・退行条件を明記する。単発の丸め差やサンプリング誤差は達成根拠にしない。
- 資源需要は同等の正常完了仕事あたり、レイテンシは同等の要求構成で評価する。成功数低下、応答省略、他処理への費用移転を成果にしない。主指標と副作用確認を分ける。
- Objectiveリンクの因果とprimaryを確認する。局所結果の改善と得点寄与の実証を分け、企業賞を主スコアに混ぜない。
- 対象・前提・目標が同じTargetの重複、実装案ごとの過分割、前提の陳腐化を確認する。実現方法や効果量の未確定だけを理由に定義可能なWhatを保留しない。
- HowになっているGoalは、元の対象と計測に戻して結果目標へ改定する。タイトルだけでなくScope/Axis/Goal/Evaluation/fingerprint/Objectiveリンクを整合させる。削減案は過去EvidenceやInterventionの仮説として保存する。

## 更新・達成判定

Writer protocolに従い直前versionを取得しCLIで更新する。actorは `skill:isucon-target`、全変更にreasonを付ける。検査のみの依頼では更新しない。Priorityは通常の管理時に[workflowのPriority](../../../tools/backlog/backlog-workflow.md#priority)に従って見直す。

目標改定では旧Scope/Axis/Goal/Evaluation/fingerprintと変更理由をHistoryに保存する。関連Interventionの寄与関係と進行中Ownerへの影響を確認し、本文や状態を勝手に変更しない。既存の実装案と独立して新しい結果目標を評価する。

現Goalに対する計測結果と担当の検証報告から達成を確認できたTargetはRESOLVEDへ移す。検査のみの依頼では遷移案を示す。Intervention採用、構造変更、候補不足、低優先度だけでは達成にしない。比較不能・指標欠損の場合は具体的不足を残すが、個別得点分離や特別な計測・再起動試験を追加の必須条件にしない。通常成果物で判定できる範囲を使う。正当性は実装・採用担当の検証を参照し、既知の違反を無視しない。

終端Targetは再開・遡及改定しない。RETIRED/MERGED/再発時の新規Targetはworkflowに従い、ACTIVE Target/Objectiveを必要とするInterventionを不整合にしない。調整が必要なら対象ID・達成根拠・必要なリンク調整と担当を残して当該状態変更を保留する。調整待ちとGoal未達を区別し、追加の性能探索で代用しない。

## 終了と引き継ぎ

件数や起票の有無だけを終了理由にしない。対象の全ACTIVE Targetについて現Goalの達成判定と状態整理を行い、ACTIVEを維持する各TargetのWhat・探索すべき問い・不足情報を説明できるところまで整理する。主要な寄与経路の既存被覆と未被覆部分も確認する。担当範囲内の保存済みEvidenceでは判定できない場合、確認した資料と具体的に不足する計測・担当報告を示す。方法探索自体や全改善余地の網羅は行わない。

Objective別の範囲、Evidenceと時点、判断理由、更新ID、未確定点を記録し、他スキルの報告には出典を付ける。analyzeへは対象範囲内でACTIVEを維持する全Target IDと探索順、その理由、対象・主指標・Goal・比較/退行条件、Objectiveへの因果、baseline参照を渡す。達成済みで状態遷移の調整待ちの場合はその旨を区別する。具体的なHowをTargetの前提条件として渡さない。Objective側の不足・修正事項は別に報告する。

RUNがあれば `docs/reports/isucon-target/` へ命名規則に従い新規保存し、なければHistoryと完了報告へ残す。最後に `task backlog -- validate` を通す。

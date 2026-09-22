# 共有Skills

`.agents/skills/`がClaude Code / Codex共通のskill実体である。`.claude/skills`はこのディレクトリへのsymlink。

## 構成

- `isucon-setup` — 初期環境・標準計測の整備と既存環境の計測補修。pprof・fgprof・nginxログ・user-transitionの収集から分析・dashboardまで確認。ベンチはユーザーが実行
- `isucon-objective` — 初期Objectiveの作成と、既存Objectiveの寄与仮説・粒度・重複・前提の検査・整理
- `isucon-target` — 既存Objectiveを起点とするTargetの探索・作成と、目標・リンク・価値・達成状況の検査・管理
- `isucon-analyze` — 指定なしなら全ACTIVE Targetの実現方法を詳細探索し、InterventionをINVESTIGATE起票
- `isucon-rethink` — 既存Targetに限定せずシステムの構造を再検討し、根拠のある改善仮説をINVESTIGATEへ直接起票
- `isucon-special-sauce` — 指定した設定資料、指定なしなら `docs/special-sources/` 全件を現行環境と照合し、適用候補をINVESTIGATEに起票
- `isucon-use-solution` — 指定した一つの `docs/solutions/` 文書を現行環境と照合し、適用候補をINVESTIGATEに起票
- `isucon-create-solution` — 再利用できる実装パターンを `docs/solutions/` に新規作成・更新。Backlog起票・実装は行わない
- `isucon-investigate` — INVESTIGATEの独立検証とREADY判定、既存TargetへのInterventionリンクの整合
- `isucon-worker` — READYの実装・適用、DOINGへ差し戻された限定修正・不採用の是正
- `isucon-verifier` — 指定RUNのベンチ後検証・採否判断。採用はVALIDATED、是正はOwnerなしDOINGでworkerへ引き渡す
- `isucon-agent` — ユーザーと対話しながら調査・分析・提案を往復で進め、合意した候補だけをINVESTIGATEへ起票する相談の入口。明示的な指示があれば対象の実装・正規deployまで行える

Objectiveの管理は `isucon-objective`、既存Objectiveに紐付くTargetの探索・管理は `isucon-target`、既存ACTIVE Targetに対する実現方法の探索は `isucon-analyze`、設定資料からの候補調査は `isucon-special-sauce`、指定solutionの適用調査は `isucon-use-solution` を使う。Interventionの起票元はanalyze・rethink・special-sauce・use-solution・agentの5スキルであり、いずれもINVESTIGATEとして `isucon-investigate` へ引き渡す。READYへの判断は `isucon-investigate` が担当し、実装・適用は通常 `isucon-worker`、ただしユーザーが明示的に指示した対象は `isucon-agent` も実装・正規deployできる。ベンチ後の検証・採否は `isucon-verifier` が担当する。`isucon-create-solution` は資料作成だけを行い、Backlogを更新しない。

Objectiveの内容・状態はisucon-objective、Targetの内容・状態とObjectiveへのリンクはisucon-targetが管理する。他スキルは既存のACTIVE TargetとObjectiveを参照し、問題・不足・達成候補を根拠付きで報告する。Backlog上の重複確認や過去調査の再利用で参照するInterventionは、現在のopen状態（INVESTIGATE／READY／DOING／VERIFY／APPLIED／BLOCKED）に限る。VALIDATED／REJECTEDは終端記録であり、これらを候補比較や重複確認の入力にしない。analyze・資料調査・investigateは担当Interventionを既存Targetへリンクできる。isucon-analyzeは引き続き既存ACTIVE Targetを起点に調査し、そのTargetへ起票時にリンクする。資料起点の調査は適合する既存Targetがなければリンクなしで起票できる。TargetリンクはREADY以降も任意だが、改善対象・目標・得点への寄与仮説・Evidence・評価条件はIntervention自身で説明する。isucon-agentが明示的なユーザー指示で実装・deployする場合も、Objective・Targetの管理と既存Ownerの権限は変更しない。

Interventionの起票とREADY判定は別の段階である。analyze・rethink・special-sauce・use-solutionは、具体的な仮説とEvidenceがあれば、Change boundaryが空または暫定のINVESTIGATEを起票できる。既存案との差分確認にChange boundaryを使うが、小規模・単一箇所・独立実装であることを起票条件にしない。isucon-investigateが調査中に範囲を具体化し、READY遷移時にHypothesis・Change boundary・Evaluationを確定する。

isucon-analyzeは達成候補の事実・snapshot・未確定点を、既存内容を保持してTargetのEvidenceへ追記できる。目標・評価条件・状態・Objectiveリンクは変更せず、達成判定はisucon-targetへ任せる。

基本の流れは `isucon-setup → isucon-objective → isucon-target → isucon-analyze → isucon-investigate → isucon-worker → 手動ベンチ → isucon-verifier`。この順序は毎回全段階を実行する要件ではなく、必要な担当スキルを呼び出す。例: `$isucon-objective 初期Objectiveを整備して`、`$isucon-target O-012のTargetを検査し、不足する対象を探索して`、`$isucon-analyze A-003`。

`isucon-agent`は対話で相談する入口である。調べる範囲を会話で決めて調査・分析・提案を往復で進め、ユーザーが合意した候補だけをINVESTIGATEへ起票する。通常は網羅探索の完了ゲートを課さず、ユーザーが終了を告げれば終わる。ユーザーが対象と範囲を明示して実装・deployを指示した場合は、AGENTS.mdとTaskfileに従って実装・正規deployまで行えるが、ベンチ実行・READY判定・Objective/Target管理・他Ownerのカード取得は行わない。担当範囲を越える段階に達したら、起票IDと残る問いを添えて後続の担当スキルを案内する。例: `$isucon-agent 今どこがスコアに効きそうか相談したい`、`$isucon-agent B-123を実装してdeployして`。

isucon-analyzeはユーザーが限定しない限り全ACTIVE Targetを詳細探索する。Priority・Evidence・前工程の推薦は調査順と各Target内の問いに使う。対象全件に詳細探索結果または既存調査を再利用できる根拠が必要で、概観だけでは完了しない。

その他の探索を担当するスキルでは、呼び出し側の親は[Work selection](../../tools/backlog/backlog-workflow.md#work-selection)に従い、全体の概観から今回の詳細探索・着手範囲を選び、根拠とともに後続担当へ渡す。単独呼び出しではそのスキルの親が担当範囲内で選ぶ。前回の結果を継続・重点変更・先に解く問いへ反映する。analyzeへの推薦は調査範囲を限定しない。

isucon-investigateは新着を含む全INVESTIGATE、isucon-workerはOwnerなしDOINGを優先し、新着を含む全READYを対象とし、ユーザーが明示的に限定した場合だけ範囲を狭める。両者はPriorityとEvidenceを着手順へ反映し、各カード処理後・終了直前にも最新一覧を確認して取得可能な対象を継続する。Owner・依存・競合と各スキルの実行制約は維持する。

`isucon-rethink`はObjectiveとシステム全体のEvidenceを判断軸に、既存Targetへ探索を限定せず構造の再検討を行う。例: `$isucon-rethink システム全体の前提を見直して改善案を探索して`。TargetリンクなしのINVESTIGATEへ直接起票し、改善仮説の独立検証・詳細化はinvestigateへ任せる。発想と現行との比較評価を分け、優先度は案の内容に応じて判断する。起票件数は求めず、候補ゼロも認める。Objective・Targetの管理や別スキルの自動起動は行わない。

資料を扱う3スキルは明示的に呼び出す。例: `$isucon-special-sauce`、`$isucon-use-solution docs/solutions/n-plus-one.md`、`$isucon-create-solution <文書化するテーマ>`。

性能、score mechanics、benchmark behavior、topology、既知solutionの観点は `isucon-analyze/references/` から必要なものだけ読む。候補調査の個別手順は各SKILL.md、三層モデルと状態遷移は共通規律に従う。

verifierの対象は指定RUNのsnapshotに限る。workerとの受け渡しと環境の排他は[workflow](../../tools/backlog/backlog-workflow.md#ownership-and-handoff)を参照する。

## 共通規律

計測基盤の整備・補修は`isucon-setup`が担当する。例: `$isucon-setup 既存環境のnginxログとpprofの計測不足を補修して`。初回構築は[初回セットアップ](isucon-setup/references/initial-setup.md)、部分補修は[setupの補修手順](isucon-setup/SKILL.md#既存環境の計測補修)を使う。計測整備のためのObjective・Target・Interventionは作らず、`isucon-worker`の実装対象にも混ぜない。

- [Backlog workflow](../../tools/backlog/backlog-workflow.md) — 三層モデル、状態遷移、READY・採用条件、優先順、書込み規則の正本
- [Backlog README](../../tools/backlog/README.md) — CLI操作、機械検査、保存形式
- [Evidence](_shared/evidence.md) — 根拠の選択・比較・欠損・因果の扱いと、寄与仮説を再評価する共通手順。判断・記録先は各スキルが定める
- [Evaluation](_shared/evaluation.md) — READY時の確認対象・採否条件の指定と、ベンチ後の比較要約

各Skillには担当範囲と実行手順を置き、共通ルールの詳細は再定義しない。担当固有の運用規則はそのSkillを正本とする。`_shared/objective-constraint-intervention.md`はworkflowへの案内として残す。

## 追加・変更

skillを追加する前に、既存スキルの責務またはreferenceで表現できるか、独立した作業段階またはユーザーが指定して使う入口が必要か確認する。共通規律は複製しない。追加・削除・改名時は、この一覧と参照元を同じ変更で更新する。全体方針や案内先が変わる場合は`AGENTS.md`も更新する。

各`SKILL.md`のfrontmatterは`name`と`description`だけを使う。詳細手順は、すべての実行に必要なものだけ本文へ置き、条件付き知識は`references/`へ分ける。

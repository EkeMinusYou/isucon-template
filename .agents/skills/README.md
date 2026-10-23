# 共有Skills

`.agents/skills/`がClaude Code / Codex共通のskill実体である。`.claude/skills`はこのディレクトリへのsymlink。

## 構成

- `isucon-setup` — 初期環境・標準計測の整備と既存環境の計測補修。pprof・nginxログ・user-transitionの収集から分析・dashboardまで確認。fgprofは任意の追加計測。ベンチを実行し、結果を確認して計測不足を補修
- `isucon-special-sauce` — 設定資料の値を現行構成に合わせて適用し、正規deploy
- `isucon-use-solution` — 指定された一つの `docs/solutions/` 文書の適用条件を現行環境と照合して報告
- `isucon-create-solution` — 再利用できる実装パターンを `docs/solutions/` に新規作成・更新
- `isucon-agent` — 調査・実装・正規deploy・ベンチ・分析を自律的に繰り返して改善する。必要な破壊的変更も許可済みとして進める
- `isucon-worktree` — app-server上の独立したCodexタスクと曜日名のworktreeへ実装を分担し、mainのベンチ・分析と重ねて、完了した候補から順次統合・計測する

設定資料を使った変更は `isucon-special-sauce`、solution文書の適用評価は `isucon-use-solution`、自律的に調査から実測・改善まで進める場合は `isucon-agent` を使います。special-sauceは資料の値を適用して正規deployし、agentは改善案を選び、実装・deploy・ベンチ・採否判断を繰り返します。

改善案は現行コード・設定、公式資料、保存済みRUNを根拠に評価し、観測事実・推論・未確定点を分けて報告する。局所指標の改善と得点への寄与を区別し、欠損成果物を0として扱わない。

初回は `isucon-setupで整備・疎通確認 → task benchで初回ベンチ → RUNの結果確認・計測補修` と進めます。その後は目的に応じた相談・資料照合、実装・正規deploy、`task bench`での計測へ進みます。依頼内容に応じて必要なスキルを呼び出します。例: `$isucon-agent 今どこがスコアに効きそうか相談したい`、`$isucon-special-sauce docs/special-sources/kernel-parameters.md`。

並行して改善を実装する場合は`isucon-worktree`を使う。app-server上の独立した通常のCodexタスクと専用worktreeへ仮説を一つずつ割り当て、調査・実装・ローカル確認・commitを任せる。mainはタスクIDで指示・進捗・結果を管理し、ベンチ・分析を重ねて、完了した候補から順次統合・計測する。新規タスクの作成条件と実装前の作業先確認は同スキルに従う。例: `$isucon-agent 独立したCodexタスクを新しく作ってworktreeで実装を分担し、mainで計測しながら改善して`。自律改善の依頼範囲ではmainが仮説選定・マージ・採否判断まで進め、ユーザーの明示的な範囲指定は優先する。実装担当にsubagentは使わず、Herdrのタブ・ペインにも依存しない。mainが`wt`で取り込み、サーバー操作と`task bench`を担当する。

`isucon-agent`への改善依頼は、必要なdeploy・ベンチ・競技DBの再作成等を含み、改善案ごとの確認を待たずに進めます。相談・調査のみなどの明示的な範囲指定は優先します。例: `$isucon-agent ベンチを回しながらスコアを改善して`、`$isucon-agent 今どこがスコアに効きそうか相談だけしたい`。

資料を扱う3スキルは明示的に呼び出す。例: `$isucon-special-sauce`、`$isucon-use-solution docs/solutions/n-plus-one.md`、`$isucon-create-solution <文書化するテーマ>`。

既知資料の適用評価は共有の[既知資料ガイド](_shared/known-solutions.md)、Evidenceの共通基準は[_shared/evidence.md](_shared/evidence.md)に従います。各Skillには担当範囲と実行手順を置きます。

## 共通規律

計測基盤の整備・補修は`isucon-setup`が担当する。例: `$isucon-setup 既存環境のnginxログとpprofの計測不足を補修して`。初回構築は[初回セットアップ](isucon-setup/references/initial-setup.md)、部分補修は[setupの補修手順](isucon-setup/SKILL.md#既存環境の計測補修)を使う。

- [Evidence](_shared/evidence.md) — 根拠の選択・比較・欠損・因果の扱い
- [既知資料ガイド](_shared/known-solutions.md) — solutionsの適用条件・追加改善の比較

各Skillには担当範囲と実行手順を置き、共通ルールの詳細は再定義しない。担当固有の運用規則はそのSkillを正本とする。

## 追加・変更

skillを追加する前に、既存スキルの責務またはreferenceで表現できるか、独立した作業段階またはユーザーが指定して使う入口が必要か確認する。共通規律は複製しない。追加・削除・改名時は、この一覧と参照元を同じ変更で更新する。全体方針や案内先が変わる場合は`AGENTS.md`も更新する。

各`SKILL.md`のfrontmatterは`name`と`description`だけを使う。詳細手順は、すべての実行に必要なものだけ本文へ置き、条件付き知識は`references/`へ分ける。

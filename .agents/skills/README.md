# 共有Skills

`.agents/skills/`がClaude Code / Codex共通のskill実体である。`.claude/skills`はこのディレクトリへのsymlink。

## 構成

- `isucon-setup` — 初期環境・標準計測の整備と既存環境の計測補修。pprof・fgprof・nginxログ・user-transitionの収集から分析・dashboardまで確認。ベンチはユーザーが実行
- `isucon-special-sauce` — 設定資料を現行環境と照合し、適用できる改善を実装・正規deploy
- `isucon-use-solution` — 指定された一つの `docs/solutions/` 文書の適用条件を現行環境と照合して報告
- `isucon-create-solution` — 再利用できる実装パターンを `docs/solutions/` に新規作成・更新
- `isucon-agent` — ユーザーと対話しながら調査・提案を進め、合意した改善案を実装・正規deployまで行う

設定資料を使った変更は `isucon-special-sauce`、solution文書の適用評価は `isucon-use-solution`、対話で調査から実装まで進める場合は `isucon-agent` を使います。special-sauceは適用可能な変更を実装して正規deployし、agentはユーザーが合意した改善案を実装・正規deployします。

改善案は現行コード・設定、公式資料、保存済みRUNを根拠に評価し、観測事実・推論・未確定点を分けて報告する。局所指標の改善と得点への寄与を区別し、欠損成果物を0として扱わない。

基本の流れは `isucon-setup → 目的に応じた相談・資料照合 → 実装・正規deploy → ユーザーによるベンチ` です。依頼内容に応じて必要なスキルを呼び出します。例: `$isucon-agent 今どこがスコアに効きそうか相談したい`、`$isucon-special-sauce docs/special-sources/kernel-parameters.md`。

`isucon-agent`は相談の入口です。調べる範囲を会話で決め、提案した改善案にユーザーが合意したらAGENTS.mdとTaskfileに従って実装・正規deployします。ベンチは実行せず、確認結果と残る問いを報告します。例: `$isucon-agent 今どこがスコアに効きそうか相談したい`、`$isucon-agent この変更を実装してdeployして`。

資料を扱う3スキルは明示的に呼び出す。例: `$isucon-special-sauce`、`$isucon-use-solution docs/solutions/n-plus-one.md`、`$isucon-create-solution <文書化するテーマ>`。

既知資料の適用評価は共有の[既知資料ガイド](_shared/known-solutions.md)、Evidenceの共通基準は[_shared/evidence.md](_shared/evidence.md)に従います。各Skillには担当範囲と実行手順を置きます。

## 共通規律

計測基盤の整備・補修は`isucon-setup`が担当する。例: `$isucon-setup 既存環境のnginxログとpprofの計測不足を補修して`。初回構築は[初回セットアップ](isucon-setup/references/initial-setup.md)、部分補修は[setupの補修手順](isucon-setup/SKILL.md#既存環境の計測補修)を使う。

- [Evidence](_shared/evidence.md) — 根拠の選択・比較・欠損・因果の扱い
- [既知資料ガイド](_shared/known-solutions.md) — special-sourcesとsolutionsの適用条件・追加改善の比較

各Skillには担当範囲と実行手順を置き、共通ルールの詳細は再定義しない。担当固有の運用規則はそのSkillを正本とする。

## 追加・変更

skillを追加する前に、既存スキルの責務またはreferenceで表現できるか、独立した作業段階またはユーザーが指定して使う入口が必要か確認する。共通規律は複製しない。追加・削除・改名時は、この一覧と参照元を同じ変更で更新する。全体方針や案内先が変わる場合は`AGENTS.md`も更新する。

各`SKILL.md`のfrontmatterは`name`と`description`だけを使う。詳細手順は、すべての実行に必要なものだけ本文へ置き、条件付き知識は`references/`へ分ける。

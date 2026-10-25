# 共有Skills

`.agents/skills/`がClaude Code / Codex共通のskill実体である。`.claude/skills`はこのディレクトリへのsymlink。

## 構成

- `isucon-setup` — 初期環境・標準計測の整備と既存環境の計測補修。pprof・nginxログ・user-transitionの収集から分析・dashboardまで確認。fgprofは任意の追加計測。ベンチを実行し、結果を確認して計測不足を補修
- `isucon-special-sauce` — 設定資料の値を現行構成に合わせて適用し、正規deploy
- `isucon-use-solution` — 指定された一つの `docs/solutions/` 文書の適用条件を現行環境と照合して報告
- `isucon-create-solution` — 再利用できる実装パターンを `docs/solutions/` に新規作成・更新
- `isucon-agent` — 調査・実装・正規deploy・ベンチ・分析を自律的に繰り返して改善する。必要な破壊的変更も許可済みとして進める

設定資料を使った変更は `isucon-special-sauce`、solution文書の適用評価は `isucon-use-solution`、自律的に調査から実測・改善まで進める場合は `isucon-agent` を使います。special-sauceは資料の値を適用して正規deployし、agentは改善案を選び、実装・deploy・ベンチ・採否判断を繰り返します。

改善を検討するときは、シナリオを解析し、その結果に基づいて仮説を立てます。現行コード・設定、公式資料、保存済みRUNを根拠に、局所指標の改善と得点への寄与を区別して評価します。観測した事実と推論、未確認点を分けて報告します。成果物の欠損は、値が0の場合と区別します。

初回は `isucon-setupで整備・疎通確認 → task benchで初回ベンチ → RUNの結果確認・計測補修` と進めます。その後は目的に応じた相談・資料照合、実装・正規deploy、`task bench`での計測へ進みます。依頼内容に応じて必要なスキルを呼び出します。例: `$isucon-agent 今どこがスコアに効きそうか相談したい`、`$isucon-special-sauce docs/special-sources/kernel-parameters.md`。

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

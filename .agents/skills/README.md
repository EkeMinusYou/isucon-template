# 共有Skills

`.agents/skills/`がClaude Code / Codex共通のskill実体である。`.claude/skills`はこのディレクトリへのsymlink。

## 構成

- `isucon-setup` — 初期環境・標準計測の整備と既存環境の計測補修。pprof・fgprof・nginxログ・user-transitionの収集から分析・dashboardまで確認。ベンチはユーザーが実行
- `isucon-analyze` — Objective・Constraintの作成・更新と、Intervention候補の発見・INVESTIGATE起票
- `isucon-special-sauce` — 指定した設定資料、指定なしなら `docs/special-sources/` 全件を現行環境と照合し、適用候補をINVESTIGATEに起票
- `isucon-use-solution` — 指定した一つの `docs/solutions/` 文書を現行環境と照合し、適用候補をINVESTIGATEに起票
- `isucon-create-solution` — 再利用できる実装パターンを `docs/solutions/` に新規作成・更新。Backlog起票・実装は行わない
- `isucon-investigate` — INVESTIGATEの独立検証とREADY判定、関連Constraint・relationの整合
- `isucon-worker` — READYの実装・適用、ベンチ後検証、rollback・復旧

広範な探索は `isucon-analyze`、設定資料からの候補調査は `isucon-special-sauce`、指定solutionの適用調査は `isucon-use-solution` を使う。Interventionの起票元はこの3スキルであり、いずれもINVESTIGATEとして `isucon-investigate` へ引き渡す。READYへの判断は `isucon-investigate` が担当し、実装・適用は `isucon-worker` が担当する。`isucon-create-solution` は資料作成だけを行い、Backlogを更新しない。

資料を扱う3スキルは明示的に呼び出す。例: `$isucon-special-sauce`、`$isucon-use-solution docs/solutions/n-plus-one.md`、`$isucon-create-solution <文書化するテーマ>`。

性能、score mechanics、benchmark behavior、topology、既知solutionの観点は `isucon-analyze/references/` から必要なものだけ読む。候補調査の個別手順は各SKILL.md、三層モデルと状態遷移は共通規律に従う。

## 共通規律

計測基盤の整備・補修は`isucon-setup`が担当する。例: `$isucon-setup 既存環境のnginxログとpprofの計測不足を補修して`。初回構築と部分補修を区別し、計測整備のためのObjective・Constraint・Interventionは作らず、`isucon-worker`の実装対象にも混ぜない。

- [Backlog workflow](../../tools/backlog/backlog-workflow.md) — 三層モデル、状態遷移、READY・採用条件、優先順、書込み規則の正本
- [Backlog README](../../tools/backlog/README.md) — CLI操作、機械検査、保存形式
- [Evidence](_shared/evidence.md) — 根拠の選択・比較・欠損・因果の扱い

各Skillには担当範囲と実行手順を置き、共通ルールの詳細は再定義しない。workerのAPPLIED上限など、担当固有の運用規則はそのSkillを正本とする。`_shared/objective-constraint-intervention.md`はworkflowへの案内として残す。

## 追加・変更

skillを追加する前に、既存スキルの責務またはreferenceで表現できるか、独立した作業段階またはユーザーが指定して使う入口が必要か確認する。共通規律は複製しない。追加・削除・改名時は、この一覧と参照元を同じ変更で更新する。全体方針や案内先が変わる場合は`AGENTS.md`も更新する。

各`SKILL.md`のfrontmatterは`name`と`description`だけを使う。詳細手順は、すべての実行に必要なものだけ本文へ置き、条件付き知識は`references/`へ分ける。

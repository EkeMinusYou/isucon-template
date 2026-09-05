# 共有Skills

`.agents/skills/`がClaude Code / Codex共通のskill実体である。`.claude/skills`はこのディレクトリへのsymlink。

## 構成

- `isucon-setup` — 競技開始時の取得、生成、build、正規deploy・bench経路の準備
- `isucon-analyze` — Objective、Constraint、Intervention候補の発見
- `isucon-special-sauce` — 指定した設定資料、指定なしなら `docs/special-sources/` 全件から適用候補を調査
- `isucon-use-solution` — 指定した一つの `docs/solutions/` 文書から適用候補を調査
- `isucon-create-solution` — 再利用できる実装パターンを `docs/solutions/` に新規作成・更新
- `isucon-investigate` — INVESTIGATEの独立検証とREADY安全ゲート
- `isucon-worker` — READYの実装・適用、ベンチ後検証、rollback・復旧

広範な探索は `isucon-analyze`、資料を指定した調査は `isucon-special-sauce` / `isucon-use-solution` を使う。候補評価は `isucon-analyze/references/known-solutions.md` と三層モデルの共通規律に従い、OwnerなしINVESTIGATEを `isucon-investigate` へ渡す。`isucon-create-solution` は資料作成だけを行い、Backlogを更新しない。

復活した3スキルは従来の明示呼び出し設定を維持する。例: `$isucon-special-sauce`、`$isucon-use-solution docs/solutions/n-plus-one.md`、`$isucon-create-solution <文書化するテーマ>`。

性能、score mechanics、benchmark behavior、topologyの観点は引き続き `isucon-analyze/references/` から必要なものだけ読む。

## 共通規律

- [Backlog workflow](../../tools/backlog/backlog-workflow.md) — 三層モデル、状態遷移、READY・採用条件、優先順、書込み規則の正本
- [Backlog README](../../tools/backlog/README.md) — CLI操作、機械検査、保存形式
- [Evidence](_shared/evidence.md) — 根拠の選択・比較・欠損・因果の扱い

各Skillには担当範囲と実行手順を置き、共通ルールの詳細は再定義しない。workerのAPPLIED上限など、担当固有の運用規則はそのSkillを正本とする。`_shared/objective-constraint-intervention.md`はworkflowへの案内として残す。

## 追加・変更

skillを追加する前に、既存スキルの責務またはreferenceで表現できるか、独立した作業段階またはユーザーが指定して使う入口が必要か確認する。共通規律は複製しない。追加・削除・改名時は、この一覧と参照元を同じ変更で更新する。全体方針や案内先が変わる場合は`AGENTS.md`も更新する。

各`SKILL.md`のfrontmatterは`name`と`description`だけを使う。詳細手順は、すべての実行に必要なものだけ本文へ置き、条件付き知識は`references/`へ分ける。

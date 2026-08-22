# 共有 Skills

このディレクトリが skill の実体です。Claude Code / Codex の両方から同じものが読まれます。

```
.agents/skills/            <- 実体（ここに追加する）
  <skill-name>/SKILL.md
.claude/skills -> ../.agents/skills   (symlink / Claude Code 用)
```

- Codex: `.agents/skills/` をプロジェクト skill として自動検出（symlink 不要）
- Claude Code: `.claude/skills/` を読むため symlink 経由で同じ実体を参照

## 追加方法

`.agents/skills/<skill-name>/SKILL.md` を作るだけ。両ツールに即反映される（セッション再起動が必要な場合あり）。

```markdown
---
name: <skill-name>
description: どんな時に使うかを1行で（この文でツールが読み込み判断する）
---

# <Skill Name>

手順や参照情報をここに書く。
補助ファイルは同じディレクトリに置き、SKILL.md から相対パスで参照する。
```

frontmatter は両ツール共通で `name` / `description` のみ使う。
ツール固有のキー（`allowed-tools` など）は片方でしか効かないので、共有前提なら避ける。

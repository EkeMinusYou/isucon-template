# 共有Skills

`.agents/skills/`がClaude Code / Codex共通のskill実体である。`.claude/skills`はこのディレクトリへのsymlink。
このテンプレートは全面再設計を試すためのもの。setupは初期ソースから外部の振る舞いを抽出し、別セッションのbootstrapはその仕様と資源条件だけからGo単体の構成を新しく設計する。

## 3段階

| 段階 | Skill | 担当と完了条件 |
|---|---|---|
| 1. 準備 | [isucon-setup](isucon-setup/SKILL.md) | 既存ソースから振る舞いを抽出し、外部仕様・提供データ・資源・接続先・ポータル操作を準備する。旧実装の計測導入・再デプロイは必須にしない |
| 2. 全面再設計 | [isucon-bootstrap](isucon-bootstrap/SKILL.md) | 外部仕様から新構成を設計・全機能を実装する。正当性・復元・標準計測・初回PASSを確認する |
| 3. 自律改善 | [isucon-agent](isucon-agent/SKILL.md) | 新実装の設計予測と実測を照合し、得点への効果から構成・実装を改善し続ける |

```text
$isucon-setup 既存ソースとHARを確認し、別セッションへ渡すdocs/behavior.mdと必要なデータ・環境を準備してください。
$isucon-bootstrap docs/behavior.mdを入力に、初期ソースを参照せずゼロから設計・実装してPASSまで検証してください。
$isucon-agent task benchでPASS・100万点を達成するまで改善してください。
```

setupはdocs/behavior.mdを保存して終了し、bootstrapは新しいセッションで開始する。bootstrapへsetupの会話やソースを引き継がない。設計・実装までならbootstrap完了で終了し、目標までの継続依頼ならbootstrapからagentへ同じセッションで進んでよい。

## 共通規律と資料

操作許可・正規経路は`AGENTS.md`、仮説・比較・欠損の扱いは[Evidence](_shared/evidence.md)を参照する。原因を絞れない場合は[診断工程](isucon-agent/SKILL.md#診断工程)を使う。
計測の技術的な確認手順は[標準計測](isucon-setup/references/measurement.md)にまとめ、bootstrapで新実装に組み込み、agentで維持する。

[秘伝のタレ](isucon-special-sauce/SKILL.md)はGoの公開HTTP/TLS・静的配信・OS・FD設定を初回ベンチ前に統合する。
[既知資料](_shared/known-solutions.md)は構成候補の参考であり、適用順序ではない。`isucon-use-solution`、`isucon-create-solution`は個別に必要な場合だけ使う。

追加・変更時は、既存の責務やreferenceで表現できるか確認する。段階固有の詳細をSKILL.mdへ重複させず、方針・案内先が変わる場合は`AGENTS.md`も更新する。

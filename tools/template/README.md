# Template backport

このリポジトリはテンプレート（`isucon-template`）のコピーとして始まる。競技中は両者が乖離するので、
大会後に「templateへ戻すべき改善」だけを取り出せるようにするための宣言とツールを置く。

| ファイル | 役割 |
| --- | --- |
| `paths.txt` | template・競技固有・除外パスの宣言 |
| `diff.sh` | `paths.txt`の`[template]`の範囲でtemplateとの差分を分類して表示する |

## 使い方

```sh
task template-diff                      # 既定で ../isucon-template と比較する
task template-diff TEMPLATE_DIR=../foo  # 別の場所のcheckoutと比較する
task template-diff DETAIL=1             # 差分の本文も表示する
```

出力は次の5枠に分かれる。

- **templateに無い** — 新規にbackportするファイル
- **内容が異なる** — 差分をbackportするファイル
- **templateにのみある** — こちらで削除済み。templateからも消すか、取り込むかを決める
- **汎用ファイルに残る競技値** — `# contest:`が付いた行。backport時に手で汎用形へ戻す
- **未分類** — `paths.txt`のどの節にも一致しないパス。節へ追記する

`[template]`に広いディレクトリを指定し、競技固有の配下を`[exclude]`で除外できます。
除外対象でも、`[template]`にファイルとして明示したパスは例外として比較対象に残ります。

## 競技固有のものを分ける3つの手段

宣言できる粒度が違うので、粗いものから順に使う。

1. **ファイルごと分ける** — `tools/contest/`、`Taskfile.contest.yml`。`paths.txt`の`[contest]`が拾う
2. **区画で囲う** — `Taskfile.yml`の`# >>> contest values >>>` 〜 `# <<< contest values <<<`。
   `diff.sh`が比較前に両側から取り除くので、競技ごとの値が差分に出ない
3. **行に印を付ける** — 汎用ファイルの中の単一の値。直前の行へ`# contest:`コメントを書く。
   自動では戻せないので、`diff.sh`が一覧として報告するだけにとどめる

区画が効くのは両側に区画がある場合だけである。templateへbackportするときは、
区画のマーカーそのものも一緒に持っていく。

## 次の競技を始めるとき

`git clone`でtemplateから競技リポジトリを作ると履歴が繋がり、`git log template/main..HEAD`でも
backport候補を出せる。コピーして新規履歴で始めた場合は共通祖先が無いので、このツールの差分が唯一の手段になる。

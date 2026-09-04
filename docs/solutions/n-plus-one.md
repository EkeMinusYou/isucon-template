# ISUCONで N+1 クエリを解消する

## 概要

一覧を1回取得したあと、一覧の各要素について関連データを1件ずつ取得する N+1 クエリを、関連データの
一括取得・集約・JOIN へ置き換える実装パターンを示す。N+1 は DB のクエリ数、コネクションの占有時間、
アプリケーションの待ち時間を親要素数に比例して増やす。

N+1 という形だけを見て機械的に修正するのではなく、親の絞り込み、関連の多重度、順序、認可、更新直後の
可視性を保ったまま読み取り経路を組み直す。過去大会での JOIN、IN、bulk、summary の例は
[`docs/resources/isucon-problem-solutions.md`](../resources/isucon-problem-solutions.md) を参照できる。

## 適用条件・制約

- [`docs/official/`](../official/) の仕様で、レスポンスの必須フィールド、順序、空データの表現、認証・認可、
  更新直後の反映、トランザクション境界を確認する。関連取得をまとめても、別ユーザーの行を混ぜたり親を
  消したりしてはいけない。
- 親を先にページング・認可・検索条件で絞る。JOIN の結果へ `LIMIT` を適用すると、関連行の数で親の件数や
  順序が変わることがある。
- 親 ID が0件のときは `IN ()` を発行しない。ID は重複排除し、SQL へ文字列連結せずパラメータ化する。
  プレースホルダー数やパケットサイズの制約を超える場合はチャンク化する。
- 一対多を複数同時に JOIN すると、親1行が関連件数の積に増える。必要な列と行だけを取得し、関連ごとの
  batch 取得や段階的な組み立てを選ぶ。
- 一括化した SQL に `WHERE`、`JOIN`、`ORDER BY` の適切なインデックスがなければ、クエリ数が減っても
  full scan や巨大な結果セットが発生する。`SELECT *` で不要な列を読み込まない。
- 在庫、予約、決済、重複防止など更新の原子性が必要なデータを、一覧のキャッシュや非正規化だけで置き換え
  ない。summary や短時間キャッシュを使う場合も、正本と更新経路を残す。
- ベンチマーカーの User-Agent を弾く、更新を捨てる、無期限に古いレスポンスを返すなど、採点条件を回避
  する実装はこのパターンに含めない。

## 探索方法

1. 負荷の大きいエンドポイントと、同じ親に対する関連クエリの呼び出しを対応づける。alp、slow query、
   CPU pprof などの既存情報は候補を絞る材料にし、SQL の同じ fingerprint だけで N+1 と断定しない。
2. コードでは、一覧を走査する `for`・`range` の内側にある `SELECT`、`Get`、`Select`、ORM の lazy load、
   `Preload`、HTTP/API 呼び出しを探す。関連キーの取得元と、レスポンスの組み立て箇所まで追う。
3. 親・子の関係、多重度、欠落時の意味、表示順、ページング、認可条件をコードと公式仕様から整理する。親の
   検索・ソート条件が関連テーブルにある場合は JOIN、親を先に絞る場合は ID 集約と一括取得を候補にする。
4. スキーマの外部キー・一意キー・複合インデックスを調べ、`WHERE` と `ORDER BY` が一括取得後も利用できる
   かを確認する。複数の一対多を一つの巨大 JOIN へまとめる前に、行数の直積を見積もる。
5. 書き込み経路、トランザクション、キャッシュ失効、read model の更新も追う。N+1 の解消をきっかけに
   キャッシュや集計済みデータを導入する場合は、正本からの再構築条件を先に定義する。

## 実装方法

### 一対多を親一覧と関連一覧に分ける

親を先に取得し、親 ID を重複排除してから `WHERE child.parent_id IN (...)` で関連をまとめて取得する。
関連を `parent_id -> []Child` の map に格納し、親の元の順序でレスポンスへ戻す。次のコードは `sqlx` を使う
代表例であり、型・列・クエリは対象アプリへ置き換える。

```go
var parents []Parent
if err := db.SelectContext(ctx, &parents, parentQuery, args...); err != nil {
	return nil, err
}

parentIDs := make([]int64, 0, len(parents))
seen := make(map[int64]struct{}, len(parents))
for _, parent := range parents {
	if _, ok := seen[parent.ID]; ok {
		continue
	}
	seen[parent.ID] = struct{}{}
	parentIDs = append(parentIDs, parent.ID)
}

childrenByParent := make(map[int64][]Child, len(parentIDs))
if len(parentIDs) > 0 {
	query, queryArgs, err := sqlx.In(
		`SELECT id, parent_id, body
		 FROM children
		 WHERE parent_id IN (?)
		 ORDER BY parent_id, id`,
		parentIDs,
	)
	if err != nil {
		return nil, err
	}

	var children []Child
	if err := db.SelectContext(ctx, &children, db.Rebind(query), queryArgs...); err != nil {
		return nil, err
	}
	for _, child := range children {
		childrenByParent[child.ParentID] = append(
			childrenByParent[child.ParentID], child,
		)
	}
}

for i := range parents {
	parents[i].Children = childrenByParent[parents[i].ID]
}
return parents, nil
```

親が多くプレースホルダー上限を超える場合は、親1件ごとのクエリへ戻さず、一定件数の ID チャンクを処理
する。チャンク間で必要な順序を保持し、各親に関連がない場合も空配列など仕様どおりの値を設定する。

### 多対一・一対一を JOIN または一括取得する

owner、category、seller のような関連を取得する場合、検索・ソート条件にも関連テーブルを使うなら適切な
JOIN で対象親を絞る。親一覧を先に確定したいなら、関連 ID を集めて `WHERE id IN (...)` で取得し map で
結びつける。

関連が存在しない状態が仕様上あり得る場合は `LEFT JOIN` や欠落時のデフォルト値を使う。`INNER JOIN` へ
変えたことで親そのものが消えないか、同じ関連 ID が複数の親に共有される場合の map の扱いも確認する。

### 親ごとの集計を GROUP BY する

コメント数、リアクション数、最新時刻などを親のループ内で `COUNT(*)` や `SUM` しない。関連キーで
集約して map 化する。

```sql
SELECT parent_id, COUNT(*) AS child_count
FROM children
WHERE parent_id IN (?, ?, ?)
GROUP BY parent_id;
```

0件の親は集計結果から消えるため、レスポンスへ戻すときは map にない親を0件として扱う。頻繁に変わらない
集計や最新状態を summary/current テーブルへ保持する場合は、更新トランザクション、書き込み競合、再計算、
複数ホストでの共有まで設計する。

### 複数の一対多とキャッシュの境界

コメントとタグのような複数の一対多は、関連ごとに batch 取得して group 化するほうが、巨大 JOIN の直積を
避けやすい。JOIN を使う場合も先に対象親を絞り、必要な列だけを読み、親のページングを JOIN 後の行数で
壊さない。

N+1 を解消した後も同じ静的マスタや集計を繰り返し読む場合だけ、短時間の cache-aside、明示的な失効、または
read model を検討する。キャッシュの正本、キー、TTL、更新後の反映、複数ホスト、再起動後の再構築を定義し、
更新直後に古い値を返さない。

### よくある誤り

- goroutine や `Promise.all` で N+1 を並列化する。クエリ数は減らず、DB とコネクションプールへの瞬間負荷
  だけが増える。
- `SELECT *` や巨大 JOIN で必要以上の行・列を返し、DB の rows sent とアプリの scan・JSON 化を増やす。
- ID の重複排除をせず、同じマスタを何度も取得する。
- preload を導入したが、ORM が実際には lazy load を続けている。
- 一括取得した関連のインデックスがなく、クエリ数の代わりに full scan が発生する。
- 一括読み取りをトランザクションへ入れたことで、更新処理とスナップショットやロック範囲が競合する。

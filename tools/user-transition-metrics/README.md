# user-transition-metrics

nginx の構造化 access log を Cookie 由来の識別子で束ね、同じ識別子が叩いた
正規化 API の連続する遷移とシナリオ群を集計するオフラインツールです。`after-bench` の
`user-transitions` digesterから呼ばれ、`runs/<RUN_ID>/user-transitions.json` を作ります。

標準計測として既定で有効です。`isucon-setup`で識別列の出力とアプリ固有のAPI分類を整えます。
識別情報がない場合は集計の有効化だけでは成立せず、RUNの必須成果物検査が失敗します。
仕様上成立しない場合は理由を残して宣言を無効化し、未設定と対象外を区別してください。

Cookie がないリクエストは遷移・シナリオのどちらにも含めません。1セッションの範囲は、
対象 Cookie が付いた最初の正規化 API から最後の正規化 API までです。login/logout の
エンドポイントを境界として要求しません。

識別子はメモリ内で SHA-256 key に変換した後のグルーピングにだけ使います。
成果物には Cookie 値、hash、ユーザー別のイベント列を保存しません。代わりに、各Cookie
セッションで観測した `METHOD + 正規化route` の集合が完全一致するものを同じシナリオ群へ
まとめます。反復回数や並行リクエストによる順序差ではシナリオを分割せず、群ごとのノード、
遷移、観測開始API、観測終了API、セッション数、継続時間を集約します。

シナリオのノードは、そのAPIがセッション内で**最初に現れた位置**も持ちます
（`first_position_avg` はセッション開始0・終了1に正規化した平均位置、
`first_offset_avg_ms` はセッション開始からの平均経過ミリ秒）。読み手はこの2つで
遷移を観測時系列順に並べられます。全ノードにこの値が揃っている場合、時系列を逆行する
遷移は戻り遷移として扱えます。この2フィールドを足した成果物は `schema_version: 3` です。

```shell
task build-user-transition-metrics
tools/user-transition-metrics/user-transition-metrics \
  -config tools/user-transition-metrics/routes.json \
  -max-scenarios 256 \
  -output /tmp/user-transitions.json \
  runs/<RUN_ID>/raw/access-*.log.zst
```

`routes.json` がアプリ固有 adapter です。`cookie_field` は access log JSON 上の
Cookieフィールド、`api_prefix` は集計対象の入口、`routes` は上から評価する
有限個の正規表現と出力名です。別のISUCONではこのファイルを差し替え、generic coreは
変更しません。未分類APIは動的IDをそのままラベルにせず、summaryへ件数だけ残します。

イベントは `msec - response_time` で求めたリクエスト開始時刻順です。同時刻は
応答終了時刻と入力順で決定し、`ambiguous_order_transitions` に数えます。次の開始が
直前の完了より早い遷移は `overlap_transitions` であり、呼び出しの因果順序を表しません。
Cookie発行前のログイン／登録は対象外であり、Cookie付きの最初のAPIが観測開始点になります。

既定上限は500万イベント、256 route、4 MiB/ログ行、64 MiB/成果物です。上限超過は
切り捨てずに失敗させ、`user-transitions.stderr` と `run.json` のartifact statusへ残します。

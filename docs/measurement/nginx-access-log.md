# 過去のnginxログとの比較

新規の標準ログは[Goアクセスログ](go-access-log.md)を使う。nginxは稼働構成に含めない。
過去RUNのJSONログは共通列の`msec`・`method`・`uri`・`status`・`response_time`・`body_bytes`から引き続き解析できる。新しい接続・TCP・上流観測がない過去ログへ値を補わない。
nginxのrequest_timeとGoのhandler時間は測る区間が異なるため、構成をまたぐ比較では区間・byte数・protocolの定義差を確認する。

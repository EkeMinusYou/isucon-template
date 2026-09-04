# Browser tools

Codex CLI からローカルの計測結果ダッシュボードを操作・撮影するための
Playwright CLI 環境です。競技サーバーには配布しません。

初回セットアップ:

```shell
task browser-install
```

スクリーンショット取得は Taskfile に定義せず、`tools/browser/` でローカルCLIを
直接使います。別ターミナルで `task dashboard` を起動してから実行してください。

```shell
cd tools/browser
npx playwright-cli -s=dashboard open http://localhost:5173/
npx playwright-cli -s=dashboard snapshot
npx playwright-cli -s=dashboard screenshot --filename ../../.task/dashboard.png --full-page
npx playwright-cli -s=dashboard close
```

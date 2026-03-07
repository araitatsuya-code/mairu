# mbox 調査メモ

`MAIRU-012 / #12` の一部として、2026年3月7日時点で Go から扱いやすい `mbox` 候補を整理したメモです。`mbox` 本実装は Phase 5 以降を前提にし、この段階では候補比較と採用方針だけを残します。

## 候補

### 1. `github.com/emersion/go-mbox`
- 参照:
  - [GitHub](https://github.com/emersion/go-mbox)
  - [pkg.go.dev](https://pkg.go.dev/github.com/emersion/go-mbox)
- 観点:
  - Reader / Writer の API が小さく、エクスポート用途の試作に向いている。
  - Go の `io.Reader` / `io.Writer` ベースで扱えるため、Mairu のストリーミング出力に載せやすい。
  - README / パッケージ説明からは「mailbox parsing / formatting」中心で、実装責務が明確。
- 注意点:
  - 変種 `mboxrd` やクライアント差異の吸収はアプリ側で検証が必要。
  - 添付ファイルを含む大きな本文を扱う場合、Gmail 側取得戦略も別途設計が要る。

### 2. `github.com/tvanriper/mbox`
- 参照:
  - [GitHub](https://github.com/tvanriper/mbox)
  - [pkg.go.dev](https://pkg.go.dev/github.com/tvanriper/mbox)
- 観点:
  - pkg.go.dev の説明では `mboxo` / `mboxrd` / `MBOXCL` / `MBOXCL2` を対象にしており、形式差異への対応幅が広い。
  - Maildir / MH / Babyl なども視野に入っているため、将来の移行アシスタント拡張とは相性が良い。
- 注意点:
  - API 面積がやや広く、Mairu の初回実装にはオーバースペック気味。
  - 形式検出やロック戦略をどこまで採用するかを先に決めないと、実装方針がぶれやすい。

## 現時点の判断

- 第1候補は `github.com/emersion/go-mbox`。
  - 理由: export 専用の最初の一歩としては API が小さく、`Mairu` 側で必要な入出力制御を持ちやすいため。
- 互換性重視の代替候補は `github.com/tvanriper/mbox`。
  - 理由: 複数方言や将来の import まで見据えるなら検討価値があるため。

これは上記ソースからの推測を含む判断です。Phase 5 着手時には、実データ 3 パターン以上で round-trip 検証してから最終決定すること。

## Phase 5 着手時の確認項目

1. Gmail から取得する本文形式を `raw` / `full` のどちらに寄せるか。
2. 添付ファイルを `mbox` へ含めるか、本文のみで割り切るか。
3. 出力対象を「重要メールのみ」にするか、「承認済み全件」まで広げるか。
4. macOS / Windows / Linux で同一 `mbox` を Thunderbird 等へ取り込めるか。

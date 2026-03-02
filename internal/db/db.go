package db

// Store は SQLite 実装の受け口となるプレースホルダ。
type Store struct{}

// OpenOptions は DB 初期化時の基本設定を表す。
type OpenOptions struct {
	Path string
}

// HealthSnapshot は DB 初期化状況の簡易確認に使う。
type HealthSnapshot struct {
	Ready bool
}

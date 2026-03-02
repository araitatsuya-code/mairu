package auth

// Client は OAuth フロー実装の受け口となるプレースホルダ。
type Client struct{}

// LoginRequest はログイン開始時に必要な入力値を表す。
type LoginRequest struct {
	RedirectURL string
	Scopes      []string
}

// LoginSession は認可画面に遷移するための一時情報を表す。
type LoginSession struct {
	State            string
	AuthorizationURL string
}

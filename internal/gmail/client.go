package gmail

import "mairu/internal/types"

// Client は Gmail API クライアント実装の受け口となるプレースホルダ。
type Client struct{}

// FetchRequest はメール取得条件の最小セットを表す。
type FetchRequest struct {
	MaxResults int
	LabelIDs   []string
}

// FetchResult は取得したメール一覧を返す。
type FetchResult struct {
	Messages []types.EmailSummary
}

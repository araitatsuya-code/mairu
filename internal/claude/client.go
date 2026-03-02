package claude

import "mairu/internal/types"

// Client は Claude API クライアント実装の受け口となるプレースホルダ。
type Client struct{}

// ClassifyRequest は分類対象メールとモデル指定をまとめる。
type ClassifyRequest struct {
	Model    string
	Messages []types.EmailSummary
}

// ClassifyResult は分類結果の受け渡しに使う。
type ClassifyResult struct {
	Results []types.ClassificationResult
}

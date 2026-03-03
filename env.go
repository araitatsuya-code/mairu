package main

import (
	"bufio"
	"fmt"
	"os"
	"strings"
)

const defaultEnvFile = ".env.local"

// loadLocalEnv は .env.local の内容を環境変数へ読み込む。
// すでに設定済みの環境変数は上書きしない。
func loadLocalEnv(path string) error {
	file, err := os.Open(path)
	if err != nil {
		if os.IsNotExist(err) {
			return nil
		}
		return fmt.Errorf("%s を開けませんでした: %w", path, err)
	}
	defer file.Close()

	scanner := bufio.NewScanner(file)
	lineNumber := 0

	for scanner.Scan() {
		lineNumber++
		line := strings.TrimSpace(scanner.Text())
		if line == "" || strings.HasPrefix(line, "#") {
			continue
		}

		if strings.HasPrefix(line, "export ") {
			line = strings.TrimSpace(strings.TrimPrefix(line, "export "))
		}

		key, value, ok := strings.Cut(line, "=")
		if !ok {
			return fmt.Errorf("%s:%d の形式が不正です", path, lineNumber)
		}

		key = strings.TrimSpace(key)
		if key == "" {
			return fmt.Errorf("%s:%d のキーが空です", path, lineNumber)
		}

		if _, exists := os.LookupEnv(key); exists {
			continue
		}

		if err := os.Setenv(key, normalizeEnvValue(value)); err != nil {
			return fmt.Errorf("%s:%d の読み込みに失敗しました: %w", path, lineNumber, err)
		}
	}

	if err := scanner.Err(); err != nil {
		return fmt.Errorf("%s の読み込みに失敗しました: %w", path, err)
	}

	return nil
}

func normalizeEnvValue(raw string) string {
	value := strings.TrimSpace(raw)
	if len(value) >= 2 {
		first := value[0]
		last := value[len(value)-1]
		if (first == '"' && last == '"') || (first == '\'' && last == '\'') {
			return value[1 : len(value)-1]
		}
	}

	return value
}

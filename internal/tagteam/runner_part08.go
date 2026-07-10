package tagteam

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"
)

func readRunPrompt(runDir, fallback string) (string, error) {
	inputPath := filepath.Join(runDir, "input.md")
	if fileExists(inputPath) {
		data, err := os.ReadFile(inputPath)
		if err != nil {
			return "", err
		}
		return string(data), nil
	}
	metaPath := filepath.Join(runDir, "meta.json")
	if fileExists(metaPath) {
		meta, err := readMeta(metaPath)
		if err != nil {
			return "", err
		}
		if strings.TrimSpace(meta.Prompt) != "" {
			return meta.Prompt, nil
		}
	}
	if strings.TrimSpace(fallback) != "" {
		return fallback, nil
	}
	return "", fmt.Errorf("run prompt not found in %s", runDir)
}

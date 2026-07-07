//go:build ignore

package main

import (
	"fmt"
	"os"
	"strings"
)

func main() {
	path := "internal/application/service/knowledge.go"
	data, err := os.ReadFile(path)
	if err != nil {
		fmt.Fprintf(os.Stderr, "read: %v\n", err)
		os.Exit(1)
	}
	src := string(data)
	count := 0

	// Add getDefaultEmbeddingModelIDFromModels helper after calculateStr
	old := `func calculateStr(strList ...string) string {
	h := md5.New()
	input := strings.Join(strList, "")
	h.Write([]byte(input))
	return hex.EncodeToString(h.Sum(nil))
}`
	new := `func calculateStr(strList ...string) string {
	h := md5.New()
	input := strings.Join(strList, "")
	h.Write([]byte(input))
	return hex.EncodeToString(h.Sum(nil))
}

// getDefaultEmbeddingModelIDFromModels returns the system default embedding model ID.
func (s *knowledgeService) getDefaultEmbeddingModelIDFromModels(ctx context.Context) (string, error) {
	models, err := s.modelService.ListModels(ctx)
	if err != nil {
		return "", fmt.Errorf("failed to list models: %w", err)
	}
	for _, m := range models {
		if m.Type == types.ModelTypeEmbedding && m.IsDefault {
			return m.ID, nil
		}
	}
	// fallback: first embedding model
	for _, m := range models {
		if m.Type == types.ModelTypeEmbedding {
			return m.ID, nil
		}
	}
	return "", fmt.Errorf("no embedding model configured")
}`
	if strings.Contains(src, old) {
		src = strings.Replace(src, old, new, 1)
		count++
	}

	if count == 0 {
		fmt.Println("No replacements made")
		os.Exit(1)
	}

	err = os.WriteFile(path, []byte(src), 0644)
	if err != nil {
		fmt.Fprintf(os.Stderr, "write: %v\n", err)
		os.Exit(1)
	}
	fmt.Printf("Done: %d replacements\n", count)
}

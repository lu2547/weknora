//go:build ignore

package main

import (
	"fmt"
	"os"
	"strings"
)

func main() {
	path := "internal/handler/initialization.go"
	data, err := os.ReadFile(path)
	if err != nil {
		fmt.Fprintf(os.Stderr, "read: %v\n", err)
		os.Exit(1)
	}
	src := string(data)
	count := 0

	// 1. Remove embedding model change check (EmbeddingModelID removed from KB)
	old := "\t// 检查Embedding模型是否可以修改\n\tif kb.EmbeddingModelID != \"\" && kb.EmbeddingModelID != req.EmbeddingModelID {\n\t\t// 检查是否已有文件\n\t\tknowledgeList, err := h.knowledgeService.ListPagedKnowledgeByKnowledgeBaseID(ctx,\n\t\t\tkbIdStr, &types.Pagination{\n\t\t\t\tPage:     1,\n\t\t\t\tPageSize: 1,\n\t\t\t}, \"\", \"\", \"\")\n\t\tif err == nil && knowledgeList != nil && knowledgeList.Total > 0 {\n\t\t\tlogger.Error(ctx, \"Cannot change embedding model when files exist\")\n\t\t\tc.Error(errors.NewBadRequestError(\"知识库中已有文件，无法修改Embedding模型\"))\n\t\t\treturn\n\t\t}\n\t}"
	new := "\t// EmbeddingModelID removed from KB (system-level default)"
	if strings.Contains(src, old) {
		src = strings.Replace(src, old, new, 1)
		count++
	}

	// 2. Fix model IDs from KB
	old = "\t// 根据知识库的模型ID获取特定模型\n\tvar models []*types.Model\n\tmodelIDs := []string{\n\t\tkb.EmbeddingModelID,\n\t\tkb.SummaryModelID,\n\t\tkb.VLMConfig.ModelID,\n\t}"
	new = "\t// Model IDs removed from KB; list default models instead\n\tvar models []*types.Model\n\tmodelIDs := []string{}"
	if strings.Contains(src, old) {
		src = strings.Replace(src, old, new, 1)
		count++
	}

	// 3. Remove multimodal/storage config section
	old = "\t// 判断多模态是否启用：有VLM模型ID或有存储配置（兼容新旧字段）\n\tstorageProvider := kb.GetStorageProvider()\n\thasMultimodal := (kb.VLMConfig.IsEnabled() ||\n\t\tkb.StorageConfig.SecretID != \"\" || kb.StorageConfig.BucketName != \"\" ||\n\t\t(storageProvider != \"\" && storageProvider != \"local\"))\n\tif config[\"multimodal\"] == nil {\n\t\tconfig[\"multimodal\"] = map[string]interface{}{\n\t\t\t\"enabled\": hasMultimodal,\n\t\t}\n\t} else {\n\t\tconfig[\"multimodal\"].(map[string]interface{})[\"enabled\"] = hasMultimodal\n\t}"
	new = "\t// Multimodal/storage config removed from KB\n\tif config[\"multimodal\"] == nil {\n\t\tconfig[\"multimodal\"] = map[string]interface{}{\n\t\t\t\"enabled\": false,\n\t\t}\n\t}"
	if strings.Contains(src, old) {
		src = strings.Replace(src, old, new, 1)
		count++
	}

	// 4. Remove storage config export block
	old = "\t\t// 添加多模态的存储配置信息（优先读新字段，兼容旧 cos_config）\n\t\teffectiveProvider := kb.GetStorageProvider()\n\t\tif kb.StorageConfig.SecretID != \"\" || (effectiveProvider != \"\" && effectiveProvider != \"local\") {\n\t\t\tif config[\"multimodal\"] == nil {\n\t\t\t\tconfig[\"multimodal\"] = map[string]interface{}{\n\t\t\t\t\t\"enabled\": true,\n\t\t\t\t}\n\t\t\t}\n\t\t\tmultimodal := config[\"multimodal\"].(map[string]interface{})\n\t\t\tmultimodal[\"storageType\"] = effectiveProvider\n\t\t\tswitch effectiveProvider {\n\t\t\tcase \"cos\":\n\t\t\t\tmultimodal[\"cos\"] = map[string]interface{}{\n\t\t\t\t\t\"secretId\":   kb.StorageConfig.SecretID,\n\t\t\t\t\t\"secretKey\":  kb.StorageConfig.SecretKey,\n\t\t\t\t\t\"region\":     kb.StorageConfig.Region,\n\t\t\t\t\t\"bucketName\": kb.StorageConfig.BucketName,\n\t\t\t\t\t\"appId\":      kb.StorageConfig.AppID,\n\t\t\t\t\t\"pathPrefix\": kb.StorageConfig.PathPrefix,\n\t\t\t\t}\n\t\t\tcase \"minio\":\n\t\t\t\tmultimodal[\"minio\"] = map[string]interface{}{\n\t\t\t\t\t\"bucketName\": kb.StorageConfig.BucketName,\n\t\t\t\t\t\"pathPrefix\": kb.StorageConfig.PathPrefix,\n\t\t\t\t}\n\t\t\t}\n\t\t}"
	new = "\t\t// Storage config removed from KB"
	if strings.Contains(src, old) {
		src = strings.Replace(src, old, new, 1)
		count++
	}

	// 5. Remove ExtractConfig export block
	old = "\tif kb.ExtractConfig != nil {\n\t\tconfig[\"nodeExtract\"] = map[string]interface{}{\n\t\t\t\"enabled\":   kb.ExtractConfig.Enabled,\n\t\t\t\"text\":      kb.ExtractConfig.Text,\n\t\t\t\"tags\":      kb.ExtractConfig.Tags,\n\t\t\t\"nodes\":     kb.ExtractConfig.Nodes,\n\t\t\t\"relations\": kb.ExtractConfig.Relations,\n\t\t}\n\t} else {\n\t\tconfig[\"nodeExtract\"] = map[string]interface{}{\n\t\t\t\"enabled\": false,\n\t\t}"
	new = "\t// ExtractConfig removed from KB\n\tconfig[\"nodeExtract\"] = map[string]interface{}{\n\t\t\"enabled\": false,"
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

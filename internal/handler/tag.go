package handler

import (
	"context"
	"net/http"

	"github.com/gin-gonic/gin"

	"github.com/Tencent/WeKnora/internal/errors"
	"github.com/Tencent/WeKnora/internal/logger"
	"github.com/Tencent/WeKnora/internal/types"
	"github.com/Tencent/WeKnora/internal/types/interfaces"
	secutils "github.com/Tencent/WeKnora/internal/utils"
)

// TagHandler handles knowledge base tag operations.
type TagHandler struct {
	tagService        interfaces.KnowledgeTagService
	tagRepo           interfaces.KnowledgeTagRepository
	chunkRepo         interfaces.ChunkRepository
	kbService         interfaces.KnowledgeBaseService
	kbShareService    interfaces.KBShareService
	agentShareService interfaces.AgentShareService
}

// DeleteTagRequest represents the request body for deleting a tag
type DeleteTagRequest struct {
	ExcludeIDs []int64 `json:"exclude_ids"` // Chunk seq_ids to exclude from deletion
}

// NewTagHandler creates a new TagHandler.
func NewTagHandler(
	tagService interfaces.KnowledgeTagService,
	tagRepo interfaces.KnowledgeTagRepository,
	chunkRepo interfaces.ChunkRepository,
	kbService interfaces.KnowledgeBaseService,
	kbShareService interfaces.KBShareService,
	agentShareService interfaces.AgentShareService,
) *TagHandler {
	return &TagHandler{tagService: tagService, tagRepo: tagRepo, chunkRepo: chunkRepo, kbService: kbService, kbShareService: kbShareService, agentShareService: agentShareService}
}

// effectiveCtxForKB validates KB access (owner or shared) and returns context with effectiveTenantID for downstream service calls.
func (h *TagHandler) effectiveCtxForKB(c *gin.Context, kbID string) (context.Context, error) {
	ctx := c.Request.Context()
	tenantID := c.GetUint64(types.TenantIDContextKey.String())
	if tenantID == 0 {
		return nil, errors.NewUnauthorizedError("Unauthorized")
	}
	userID, userExists := c.Get(types.UserIDContextKey.String())
	kbID = secutils.SanitizeForLog(kbID)
	if kbID == "" {
		return nil, errors.NewBadRequestError("Knowledge base ID cannot be empty")
	}
	_, err := h.kbService.GetKnowledgeBaseByID(ctx, kbID)
	if err != nil {
		logger.ErrorWithFields(ctx, err, nil)
		return nil, errors.NewInternalServerError(err.Error())
	}
	// KB found — grant access with current tenant context
	effCtx := context.WithValue(ctx, types.TenantIDContextKey, tenantID)
	if userExists && h.kbShareService != nil {
		permission, isShared, permErr := h.kbShareService.CheckUserKBPermission(ctx, kbID, userID.(string))
		if permErr == nil && isShared {
			sourceTenantID, srcErr := h.kbShareService.GetKBSourceTenant(ctx, kbID)
			if srcErr == nil {
				logger.Infof(ctx, "User %s accessing shared KB %s with permission %s, source tenant: %d",
					userID.(string), kbID, permission, sourceTenantID)
				return context.WithValue(ctx, types.TenantIDContextKey, sourceTenantID), nil
			}
		}
	}
	return effCtx, nil
}

// resolveTagID resolves tag_id parameter which can be either UUID or seq_id (integer).
// Uses tenant from c's context. Use resolveTagIDWithCtx when effectiveTenantID is set (e.g. shared KB).
func (h *TagHandler) resolveTagID(c *gin.Context) (string, error) {
	return h.resolveTagIDWithCtx(c, c.Request.Context())
}

// resolveTagIDWithCtx resolves tag_id using the given context for tenant (e.g. effCtx for shared KB).
// Since SeqID is removed from tags, this now just returns the tag_id param directly (UUID).
func (h *TagHandler) resolveTagIDWithCtx(c *gin.Context, ctx context.Context) (string, error) {
	tagIDParam := secutils.SanitizeForLog(c.Param("tag_id"))
	return tagIDParam, nil
}

// getChunksBySeqIDs retrieves chunks by their seq_ids.
func (h *TagHandler) getChunksBySeqIDs(ctx context.Context, seqIDs []int64) ([]*types.Chunk, error) {
	return h.chunkRepo.ListChunksBySeqID(ctx, seqIDs)
}

// ListTags godoc
// @Summary      获取标签列表
// @Description  获取知识库下的所有标签及统计信息
// @Tags         标签管理
// @Accept       json
// @Produce      json
// @Param        id         path      string  true   "知识库ID"
// @Param        page       query     int     false  "页码"
// @Param        page_size  query     int     false  "每页数量"
// @Param        keyword    query     string  false  "关键词搜索"
// @Success      200        {object}  map[string]interface{}  "标签列表"
// @Failure      400        {object}  errors.AppError         "请求参数错误"
// @Security     Bearer
// @Security     ApiKeyAuth
// @Router       /knowledge-bases/{id}/tags [get]
func (h *TagHandler) ListTags(c *gin.Context) {
	ctx := c.Request.Context()
	kbID := secutils.SanitizeForLog(c.Param("id"))

	effCtx, err := h.effectiveCtxForKB(c, kbID)
	if err != nil {
		c.Error(err)
		return
	}

	var page types.Pagination
	if err := c.ShouldBindQuery(&page); err != nil {
		logger.Error(ctx, "Failed to bind pagination query", err)
		c.Error(errors.NewBadRequestError("分页参数不合法").WithDetails(err.Error()))
		return
	}

	keyword := secutils.SanitizeForLog(c.Query("keyword"))

	tags, err := h.tagService.ListTags(effCtx, kbID, &page, keyword)
	if err != nil {
		logger.ErrorWithFields(ctx, err, nil)
		c.Error(err)
		return
	}

	c.JSON(http.StatusOK, gin.H{
		"success": true,
		"data":    tags,
	})
}

type createTagRequest struct {
	Name      string `json:"name"       binding:"required"`
	ParentID  string `json:"parent_id"`
	Color     string `json:"color"`
	SortOrder int    `json:"sort_order"`
}

// CreateTag godoc
// @Summary      创建标签
// @Description  在知识库下创建新标签
// @Tags         标签管理
// @Accept       json
// @Produce      json
// @Param        id       path      string  true  "知识库ID"
// @Param        request  body      object{name=string,color=string,sort_order=int}  true  "标签信息"
// @Success      200      {object}  map[string]interface{}  "创建的标签"
// @Failure      400      {object}  errors.AppError         "请求参数错误"
// @Security     Bearer
// @Security     ApiKeyAuth
// @Router       /knowledge-bases/{id}/tags [post]
func (h *TagHandler) CreateTag(c *gin.Context) {
	ctx := c.Request.Context()
	kbID := secutils.SanitizeForLog(c.Param("id"))

	effCtx, err := h.effectiveCtxForKB(c, kbID)
	if err != nil {
		c.Error(err)
		return
	}

	var req createTagRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		logger.Error(ctx, "Failed to bind create tag payload", err)
		c.Error(errors.NewBadRequestError("请求参数不合法").WithDetails(err.Error()))
		return
	}

	tag, err := h.tagService.CreateTag(effCtx, kbID,
		secutils.SanitizeForLog(req.ParentID),
		secutils.SanitizeForLog(req.Name), req.SortOrder)
	if err != nil {
		logger.ErrorWithFields(ctx, err, map[string]interface{}{
			"kb_id": kbID,
		})
		c.Error(err)
		return
	}

	c.JSON(http.StatusOK, gin.H{
		"success": true,
		"data":    tag,
	})
}

type updateTagRequest struct {
	Name      *string `json:"name"`
	Color     *string `json:"color"`
	SortOrder *int    `json:"sort_order"`
}

// UpdateTag godoc
// @Summary      更新标签
// @Description  更新标签信息
// @Tags         标签管理
// @Accept       json
// @Produce      json
// @Param        id       path      string  true  "知识库ID"
// @Param        tag_id   path      string  true  "标签ID (UUID或seq_id)"
// @Param        request  body      object  true  "标签更新信息"
// @Success      200      {object}  map[string]interface{}  "更新后的标签"
// @Failure      400      {object}  errors.AppError         "请求参数错误"
// @Security     Bearer
// @Security     ApiKeyAuth
// @Router       /knowledge-bases/{id}/tags/{tag_id} [put]
func (h *TagHandler) UpdateTag(c *gin.Context) {
	ctx := c.Request.Context()
	kbID := secutils.SanitizeForLog(c.Param("id"))

	effCtx, err := h.effectiveCtxForKB(c, kbID)
	if err != nil {
		c.Error(err)
		return
	}

	tagID, err := h.resolveTagIDWithCtx(c, effCtx)
	if err != nil {
		c.Error(err)
		return
	}

	var req updateTagRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		logger.Error(ctx, "Failed to bind update tag payload", err)
		c.Error(errors.NewBadRequestError("请求参数不合法").WithDetails(err.Error()))
		return
	}

	tag, err := h.tagService.UpdateTag(effCtx, tagID, req.Name, req.SortOrder)
	if err != nil {
		logger.ErrorWithFields(ctx, err, map[string]interface{}{
			"tag_id": tagID,
		})
		c.Error(err)
		return
	}

	c.JSON(http.StatusOK, gin.H{
		"success": true,
		"data":    tag,
	})
}

// DeleteTag godoc
// @Summary      删除标签
// @Description  删除标签，可使用force=true强制删除被引用的标签，content_only=true仅删除标签下的内容而保留标签本身
// @Tags         标签管理
// @Accept       json
// @Produce      json
// @Param        id            path      string              true   "知识库ID"
// @Param        tag_id        path      string              true   "标签ID (UUID或seq_id)"
// @Param        force         query     bool                false  "强制删除"
// @Param        content_only  query     bool                false  "仅删除内容，保留标签"
// @Param        body          body      DeleteTagRequest    false  "删除选项"
// @Success      200           {object}  map[string]interface{}  "删除成功"
// @Failure      400           {object}  errors.AppError         "请求参数错误"
// @Security     Bearer
// @Security     ApiKeyAuth
// @Router       /knowledge-bases/{id}/tags/{tag_id} [delete]
func (h *TagHandler) DeleteTag(c *gin.Context) {
	ctx := c.Request.Context()
	kbID := secutils.SanitizeForLog(c.Param("id"))

	effCtx, err := h.effectiveCtxForKB(c, kbID)
	if err != nil {
		c.Error(err)
		return
	}

	tagID, err := h.resolveTagIDWithCtx(c, effCtx)
	if err != nil {
		c.Error(err)
		return
	}

	force := c.Query("force") == "true"
	contentOnly := c.Query("content_only") == "true"

	var req DeleteTagRequest
	_ = c.ShouldBindJSON(&req)

	var excludeUUIDs []string
	if len(req.ExcludeIDs) > 0 {
		chunks, err := h.getChunksBySeqIDs(effCtx, req.ExcludeIDs)
		if err != nil {
			logger.Warnf(ctx, "Failed to resolve exclude_ids: %v", err)
		} else {
			excludeUUIDs = make([]string, len(chunks))
			for i, chunk := range chunks {
				excludeUUIDs[i] = chunk.ID
			}
		}
	}

	if err := h.tagService.DeleteTag(effCtx, tagID, force, contentOnly, excludeUUIDs); err != nil {
		logger.ErrorWithFields(ctx, err, map[string]interface{}{
			"tag_id": tagID,
		})
		c.Error(err)
		return
	}

	c.JSON(http.StatusOK, gin.H{
		"success": true,
	})
}

// NOTE: TagHandler currently exposes CRUD for tags and statistics.
// Knowledge / Chunk tagging is handled via dedicated knowledge and FAQ APIs.

// ListTagTree godoc
// @Summary      获取标签树
// @Description  获取知识库下的标签树形结构，包含每个节点的统计信息
// @Tags         标签管理
// @Accept       json
// @Produce      json
// @Param        id   path      string  true  "知识库ID"
// @Success      200  {object}  map[string]interface{}  "标签树"
// @Failure      400  {object}  errors.AppError         "请求参数错误"
// @Security     Bearer
// @Security     ApiKeyAuth
// @Router       /knowledge-bases/{id}/tag-tree [get]
func (h *TagHandler) ListTagTree(c *gin.Context) {
	ctx := c.Request.Context()
	kbID := secutils.SanitizeForLog(c.Param("id"))

	effCtx, err := h.effectiveCtxForKB(c, kbID)
	if err != nil {
		c.Error(err)
		return
	}

	tree, err := h.tagService.ListTagTree(effCtx, kbID)
	if err != nil {
		logger.ErrorWithFields(ctx, err, map[string]interface{}{"kb_id": kbID})
		c.Error(err)
		return
	}
	c.JSON(http.StatusOK, gin.H{
		"success": true,
		"data":    tree,
	})
}

type moveTagRequest struct {
	// NewParentID is the id of the new parent; empty string means move to root.
	NewParentID string `json:"new_parent_id"`
}

// MoveTag godoc
// @Summary      移动标签
// @Description  将标签移动到新的父标签下（传空字符串表示移至根节点）
// @Tags         标签管理
// @Accept       json
// @Produce      json
// @Param        id       path      string  true  "知识库ID"
// @Param        tag_id   path      string  true  "标签ID (UUID或seq_id)"
// @Param        request  body      moveTagRequest  true  "移动信息"
// @Success      200      {object}  map[string]interface{}  "移动后的标签"
// @Failure      400      {object}  errors.AppError         "请求参数错误"
// @Security     Bearer
// @Security     ApiKeyAuth
// @Router       /knowledge-bases/{id}/tags/{tag_id}/move [post]
func (h *TagHandler) MoveTag(c *gin.Context) {
	ctx := c.Request.Context()
	kbID := secutils.SanitizeForLog(c.Param("id"))

	effCtx, err := h.effectiveCtxForKB(c, kbID)
	if err != nil {
		c.Error(err)
		return
	}

	tagID, err := h.resolveTagIDWithCtx(c, effCtx)
	if err != nil {
		c.Error(err)
		return
	}

	var req moveTagRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		logger.Error(ctx, "Failed to bind move tag payload", err)
		c.Error(errors.NewBadRequestError("请求参数不合法").WithDetails(err.Error()))
		return
	}

	// NewParentID is now always a UUID
	newParentID := secutils.SanitizeForLog(req.NewParentID)

	tag, err := h.tagService.MoveTag(effCtx, tagID, newParentID)
	if err != nil {
		logger.ErrorWithFields(ctx, err, map[string]interface{}{"tag_id": tagID})
		c.Error(err)
		return
	}
	c.JSON(http.StatusOK, gin.H{
		"success": true,
		"data":    tag,
	})
}

// shareTagRequest represents the request body for sharing a tag.
type shareTagRequest struct {
	GroupKey   string `json:"group_key"  binding:"required"`
	Permission string `json:"permission"`
}

// resolveTagIDFromParam resolves the :tag_id path param to the underlying UUID,
// without requiring KB context (used by /knowledge-tags/:tag_id/* routes).
func (h *TagHandler) resolveTagIDFromParam(c *gin.Context) (string, *types.KnowledgeTag, error) {
	tagIDParam := secutils.SanitizeForLog(c.Param("tag_id"))
	tenantID := c.GetUint64(types.TenantIDContextKey.String())
	if tenantID == 0 {
		return "", nil, errors.NewUnauthorizedError("Unauthorized")
	}
	ctx := context.WithValue(c.Request.Context(), types.TenantIDContextKey, tenantID)
	// tag_id is always UUID now
	tag, err := h.tagRepo.GetByID(ctx, tagIDParam)
	if err != nil {
		return "", nil, errors.NewNotFoundError("标签不存在")
	}
	return tag.ID, tag, nil
}

// CreateTagShare godoc
// @Summary      共享标签给组织
// @Description  将标签共享给指定组织/群组，仅标签所在知识库的 owner 可操作
// @Tags         标签共享
// @Accept       json
// @Produce      json
// @Param        tag_id   path      string  true  "标签ID (UUID 或 seq_id)"
// @Param        request  body      object{group_key=string,permission=string}  true  "共享信息"
// @Success      200      {object}  map[string]interface{}
// @Failure      400      {object}  errors.AppError
// @Security     Bearer
// @Security     ApiKeyAuth
// @Router       /knowledge-tags/{tag_id}/shares [post]
func (h *TagHandler) CreateTagShare(c *gin.Context) {
	ctx := c.Request.Context()
	tenantID := c.GetUint64(types.TenantIDContextKey.String())
	if tenantID == 0 {
		c.Error(errors.NewUnauthorizedError("Unauthorized"))
		return
	}
	userIDVal, _ := c.Get(types.UserIDContextKey.String())
	userID, _ := userIDVal.(string)

	tagID, _, err := h.resolveTagIDFromParam(c)
	if err != nil {
		c.Error(err)
		return
	}

	var req shareTagRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		logger.Error(ctx, "Failed to bind share tag payload", err)
		c.Error(errors.NewBadRequestError("请求参数不合法").WithDetails(err.Error()))
		return
	}

	effCtx := context.WithValue(ctx, types.TenantIDContextKey, tenantID)
	share, err := h.tagService.ShareTag(effCtx, tagID,
		secutils.SanitizeForLog(req.GroupKey), userID,
		secutils.SanitizeForLog(req.Permission))
	if err != nil {
		logger.ErrorWithFields(ctx, err, map[string]interface{}{"tag_id": tagID})
		c.Error(err)
		return
	}
	c.JSON(http.StatusOK, gin.H{
		"success": true,
		"data":    share,
	})
}

// ListTagShares godoc
// @Summary      获取标签共享列表
// @Description  列出标签的所有共享记录
// @Tags         标签共享
// @Produce      json
// @Param        tag_id  path      string  true  "标签ID"
// @Success      200     {object}  map[string]interface{}
// @Failure      400     {object}  errors.AppError
// @Security     Bearer
// @Security     ApiKeyAuth
// @Router       /knowledge-tags/{tag_id}/shares [get]
func (h *TagHandler) ListTagShares(c *gin.Context) {
	ctx := c.Request.Context()
	tenantID := c.GetUint64(types.TenantIDContextKey.String())
	if tenantID == 0 {
		c.Error(errors.NewUnauthorizedError("Unauthorized"))
		return
	}
	tagID, _, err := h.resolveTagIDFromParam(c)
	if err != nil {
		c.Error(err)
		return
	}
	effCtx := context.WithValue(ctx, types.TenantIDContextKey, tenantID)
	shares, err := h.tagService.ListTagShares(effCtx, tagID)
	if err != nil {
		logger.ErrorWithFields(ctx, err, map[string]interface{}{"tag_id": tagID})
		c.Error(err)
		return
	}
	c.JSON(http.StatusOK, gin.H{
		"success": true,
		"data":    shares,
	})
}

// DeleteTagShare godoc
// @Summary      取消标签共享
// @Description  删除一条标签共享记录
// @Tags         标签共享
// @Produce      json
// @Param        tag_id    path      string  true  "标签ID"
// @Param        share_id  path      string  true  "共享记录ID"
// @Success      200       {object}  map[string]interface{}
// @Failure      400       {object}  errors.AppError
// @Security     Bearer
// @Security     ApiKeyAuth
// @Router       /knowledge-tags/{tag_id}/shares/{share_id} [delete]
func (h *TagHandler) DeleteTagShare(c *gin.Context) {
	ctx := c.Request.Context()
	tenantID := c.GetUint64(types.TenantIDContextKey.String())
	if tenantID == 0 {
		c.Error(errors.NewUnauthorizedError("Unauthorized"))
		return
	}
	userIDVal, _ := c.Get(types.UserIDContextKey.String())
	userID, _ := userIDVal.(string)
	shareID := secutils.SanitizeForLog(c.Param("share_id"))
	if shareID == "" {
		c.Error(errors.NewBadRequestError("共享ID不能为空"))
		return
	}
	effCtx := context.WithValue(ctx, types.TenantIDContextKey, tenantID)
	if err := h.tagService.RevokeTagShare(effCtx, shareID, userID); err != nil {
		logger.ErrorWithFields(ctx, err, map[string]interface{}{"share_id": shareID})
		c.Error(err)
		return
	}
	c.JSON(http.StatusOK, gin.H{"success": true})
}

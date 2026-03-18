<template>
  <div class="popup-content">
    <div class="popup-content-wrapper">
      <div v-if="content" class="full-content" :class="{ 'html-content': isHtml }">
        <div v-if="isHtml" v-html="processedContent"></div>
        <template v-else>{{ content }}</template>
      </div>
    </div>
    <div v-if="hasInfo" class="info-section">
      <!-- 文档名行 -->
      <div v-if="knowledgeTitle" class="info-row info-row--doc">
        <div class="doc-info">
          <div class="doc-name">{{ knowledgeTitle }}</div>
          <div v-if="knowledgeBaseName" class="doc-meta">
            <span class="doc-kbname">{{ knowledgeBaseName }}</span>
            <template v-if="tagName">
              <span class="doc-sep"> · </span>
              <span class="doc-tag">{{ tagName }}</span>
            </template>
          </div>
        </div>
        <button
          v-if="knowledgeId"
          class="doc-preview-btn"
          :title="$t('chat.previewDocument')"
          @click.stop="emit('preview', { knowledgeId, knowledgeTitle, knowledgeBaseName, tagName })"
        >
          <t-icon name="browse" size="14px" />
        </button>
      </div>
    </div>
  </div>
</template>

<script setup lang="ts">
import { computed } from 'vue';
import { sanitizeHTML } from '@/utils/security';

interface Props {
  content?: string;
  chunkId?: string;
  knowledgeId?: string;
  knowledgeTitle?: string;
  knowledgeBaseName?: string;
  tagName?: string;
  isHtml?: boolean; // 是否以 HTML 格式显示内容
}

const props = defineProps<Props>();
const emit = defineEmits<{
  (e: 'preview', info: { knowledgeId: string; knowledgeTitle?: string; knowledgeBaseName?: string; tagName?: string }): void;
}>();

const hasInfo = computed(() => {
  return !!(props.knowledgeTitle || props.knowledgeId);
});

// 处理 HTML 内容
const processedContent = computed(() => {
  if (!props.content) return '';
  if (props.isHtml) {
    return sanitizeHTML(props.content);
  }
  return props.content;
});
</script>

<style lang="less" scoped>
.popup-content {
  display: flex;
  flex-direction: column;
  max-height: 400px;
  max-width: 500px;
  border: 1px solid var(--td-brand-color);
  border-radius: 4px;
  word-wrap: break-word;
  word-break: break-word;
  overflow: hidden;
  
  .popup-content-wrapper {
    flex: 1;
    overflow-y: auto;
    overflow-x: hidden;
    padding: 12px;
    min-height: 0;
  }
  
  .full-content {
    font-size: 13px;
    color: var(--td-text-color-primary);
    line-height: 1.8;
    white-space: pre-wrap;
    word-break: break-word;
    
    &.html-content {
      white-space: normal;
      
      :deep(p) {
        margin: 8px 0;
        line-height: 1.8;
      }
      
      :deep(br) {
        line-height: 1.8;
      }
    }
  }
  
  .info-section {
    flex-shrink: 0;
    padding: 8px 12px;
    border-top: 1px solid var(--td-component-stroke);
    background: var(--td-bg-color-secondarycontainer);
  }
  
  .info-row {
    display: flex;
    align-items: center;
    gap: 8px;
  }
  
  .info-row--doc {
    justify-content: space-between;
  }
  
  .doc-info {
    flex: 1;
    min-width: 0;
    display: flex;
    flex-direction: column;
    gap: 2px;
  }
  
  .doc-name {
    font-size: 12px;
    font-weight: 600;
    color: var(--td-text-color-primary);
    overflow: hidden;
    text-overflow: ellipsis;
    white-space: nowrap;
  }
  
  .doc-meta {
    font-size: 11px;
    display: flex;
    align-items: center;
    gap: 2px;
    flex-wrap: wrap;
  }
  
  .doc-kbname {
    color: var(--td-brand-color);
  }
  
  .doc-sep {
    color: var(--td-text-color-placeholder);
  }
  
  .doc-tag {
    background: var(--td-brand-color-light);
    color: var(--td-brand-color);
    padding: 0px 5px;
    border-radius: 3px;
    font-size: 10px;
  }
  
  .doc-preview-btn {
    flex-shrink: 0;
    display: flex;
    align-items: center;
    justify-content: center;
    width: 24px;
    height: 24px;
    border: 1px solid var(--td-component-stroke);
    border-radius: 4px;
    background: var(--td-bg-color-container);
    color: var(--td-text-color-secondary);
    cursor: pointer;
    transition: background 0.15s, color 0.15s, border-color 0.15s;
    
    &:hover {
      background: var(--td-brand-color-light);
      color: var(--td-brand-color);
      border-color: var(--td-brand-color);
    }
  }
}
</style>


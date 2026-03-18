<template>
  <Teleport to="body">
    <Transition name="doc-preview-drawer">
      <div
        v-if="visible"
        class="doc-preview-overlay"
        @click.self="close"
      >
        <div class="doc-preview-drawer">
          <!-- Header -->
          <div class="doc-preview-header">
            <div class="doc-preview-header-info">
              <div class="doc-preview-title" :title="knowledgeTitle">
                <t-icon name="file" size="16px" class="doc-preview-title-icon" />
                <span>{{ knowledgeTitle || $t('chat.unknownDocument') }}</span>
              </div>
              <div v-if="knowledgeBaseName" class="doc-preview-subtitle">
                <t-icon name="books" size="13px" class="subtitle-icon" />
                <span>{{ knowledgeBaseName }}</span>
                <template v-if="tagName">
                  <span class="subtitle-sep"> · </span>
                  <t-icon name="tag" size="12px" class="subtitle-icon tag-icon" />
                  <span class="tag-badge">{{ tagName }}</span>
                </template>
              </div>
            </div>
            <button type="button" class="doc-preview-close" @click="close" :aria-label="$t('common.close')">
              <t-icon name="close" size="18px" />
            </button>
          </div>

          <!-- Body: document preview -->
          <div class="doc-preview-body">
            <div v-if="knowledgeId && visible && !resolvedFileType" class="doc-preview-loading">
              <t-loading size="medium" />
            </div>
            <DocumentPreview
              v-else-if="knowledgeId && visible && resolvedFileType"
              :knowledgeId="knowledgeId"
              :fileType="resolvedFileType"
              :fileName="resolvedTitle || knowledgeTitle || ''"
              :active="visible"
            />
            <div v-else-if="!knowledgeId" class="doc-preview-empty">
              {{ $t('chat.noDocumentToPreview') }}
            </div>
          </div>
        </div>
      </div>
    </Transition>
  </Teleport>
</template>

<script setup lang="ts">
import { ref, watch } from 'vue';
import { useI18n } from 'vue-i18n';
import DocumentPreview from '@/components/document-preview.vue';
import { getKnowledgeDetails } from '@/api/knowledge-base';

const { t } = useI18n();

const props = defineProps<{
  visible: boolean;
  knowledgeId?: string;
  knowledgeTitle?: string;
  knowledgeBaseName?: string;
  tagName?: string;
  fileType?: string;
}>();

const emit = defineEmits<{
  (e: 'update:visible', value: boolean): void;
}>();

// 实际使用的 fileType，可能通过 API 加载
const resolvedFileType = ref(props.fileType || '');
const resolvedTitle = ref(props.knowledgeTitle || '');

watch(
  () => [props.visible, props.knowledgeId, props.fileType] as [boolean, string|undefined, string|undefined],
  async ([visible, knowledgeId, fileType]) => {
    if (!visible || !knowledgeId) return;
    // 如果 fileType 已有，直接使用
    if (fileType) {
      resolvedFileType.value = fileType;
      resolvedTitle.value = props.knowledgeTitle || '';
      return;
    }
    // 否则通过 API 获取 knowledge 详情
    try {
      const resp = await getKnowledgeDetails(knowledgeId);
      const data = resp?.data;
      if (data) {
        resolvedFileType.value = data.file_type || '';
        resolvedTitle.value = props.knowledgeTitle || data.title || data.file_name || '';
      }
    } catch (e) {
      // ignore, will show unsupported
    }
  },
  { immediate: true }
);

const close = () => {
  emit('update:visible', false);
};
</script>

<style lang="less" scoped>
.doc-preview-overlay {
  position: fixed;
  top: 0;
  left: 0;
  right: 0;
  bottom: 0;
  background: rgba(0, 0, 0, 0.35);
  z-index: 9000;
  display: flex;
  justify-content: flex-end;
}

.doc-preview-drawer {
  width: 50vw;
  min-width: 480px;
  max-width: 900px;
  height: 100%;
  background: var(--td-bg-color-container);
  box-shadow: -4px 0 32px rgba(0, 0, 0, 0.18);
  display: flex;
  flex-direction: column;
  overflow: hidden;
}

.doc-preview-header {
  display: flex;
  align-items: flex-start;
  justify-content: space-between;
  padding: 16px 20px;
  border-bottom: 1px solid var(--td-component-stroke);
  flex-shrink: 0;
  gap: 12px;
}

.doc-preview-header-info {
  flex: 1;
  min-width: 0;
  display: flex;
  flex-direction: column;
  gap: 4px;
}

.doc-preview-title {
  display: flex;
  align-items: center;
  gap: 6px;
  font-size: 15px;
  font-weight: 600;
  color: var(--td-text-color-primary);
  
  span {
    overflow: hidden;
    text-overflow: ellipsis;
    white-space: nowrap;
  }
  
  .doc-preview-title-icon {
    flex-shrink: 0;
    color: var(--td-brand-color);
  }
}

.doc-preview-subtitle {
  display: flex;
  align-items: center;
  gap: 4px;
  font-size: 12px;
  color: var(--td-text-color-secondary);
  
  .subtitle-icon {
    flex-shrink: 0;
    color: var(--td-text-color-placeholder);
  }
  
  .tag-icon {
    color: var(--td-brand-color);
    opacity: 0.8;
  }
  
  .subtitle-sep {
    color: var(--td-text-color-placeholder);
  }
  
  .tag-badge {
    background: var(--td-brand-color-light);
    color: var(--td-brand-color);
    padding: 1px 6px;
    border-radius: 3px;
    font-size: 11px;
  }
}

.doc-preview-close {
  flex-shrink: 0;
  display: flex;
  align-items: center;
  justify-content: center;
  width: 28px;
  height: 28px;
  border: none;
  border-radius: 4px;
  background: transparent;
  color: var(--td-text-color-secondary);
  cursor: pointer;
  transition: background 0.15s, color 0.15s;
  
  &:hover {
    background: var(--td-bg-color-secondarycontainer);
    color: var(--td-text-color-primary);
  }
}

.doc-preview-body {
  flex: 1;
  overflow: hidden;
  display: flex;
  flex-direction: column;
  padding: 16px 20px;
  
  :deep(.document-preview) {
    flex: 1;
    min-height: 0;
    height: 100%;
  }
}

.doc-preview-loading {
  display: flex;
  align-items: center;
  justify-content: center;
  flex: 1;
  color: var(--td-text-color-secondary);
}

.doc-preview-empty {
  display: flex;
  align-items: center;
  justify-content: center;
  flex: 1;
  color: var(--td-text-color-placeholder);
  font-size: 14px;
}

// Transition animation
.doc-preview-drawer-enter-active,
.doc-preview-drawer-leave-active {
  transition: opacity 0.25s ease;
  
  .doc-preview-drawer {
    transition: transform 0.25s ease;
  }
}

.doc-preview-drawer-enter-from,
.doc-preview-drawer-leave-to {
  opacity: 0;
  
  .doc-preview-drawer {
    transform: translateX(100%);
  }
}
</style>

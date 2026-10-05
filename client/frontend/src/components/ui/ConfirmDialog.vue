<template>
  <Modal
    :visible="visible"
    width="400px"
    :aria-label="title"
    :show-close="false"
    :close-on-overlay="false"
    @close="handleCancel"
  >
    <div class="confirm-header">
      <div class="confirm-icon" :class="`icon-${type}`">
        <DynamicIcon :name="iconName" :size="24" />
      </div>
      <h3 class="confirm-title">{{ title }}</h3>
      <p class="confirm-message">{{ message }}</p>
    </div>
    <template #footer>
      <div class="confirm-footer">
        <button class="btn btn-cancel" @click="handleCancel">
          {{ cancelText }}
        </button>
        <button class="btn" :class="`btn-${type}`" @click="handleConfirm">
          {{ confirmText }}
        </button>
      </div>
    </template>
  </Modal>
</template>

<script setup>
import { computed } from 'vue'
import DynamicIcon from './DynamicIcon.vue'
import Modal from '../feedback/Modal.vue'

const props = defineProps({
  visible: {
    type: Boolean,
    default: false
  },
  type: {
    type: String,
    default: 'warning',
    validator: (v) => ['info', 'success', 'warning', 'danger'].includes(v)
  },
  title: {
    type: String,
    default: '确认'
  },
  message: {
    type: String,
    default: '确定执行此操作？'
  },
  confirmText: {
    type: String,
    default: '确定'
  },
  cancelText: {
    type: String,
    default: '取消'
  }
})

const emit = defineEmits(['confirm', 'cancel'])

const iconMap = {
  info: 'info',
  success: 'success',
  warning: 'warning',
  danger: 'error'
}

const iconName = computed(() => iconMap[props.type])

function handleConfirm() {
  emit('confirm')
}

function handleCancel() {
  emit('cancel')
}
</script>

<style scoped>
.confirm-header {
  padding: 8px 0 0;
  text-align: center;
}

.confirm-icon {
  width: 56px;
  height: 56px;
  border-radius: 50%;
  display: flex;
  align-items: center;
  justify-content: center;
  margin: 0 auto 16px;
}

.icon-info {
  background: var(--color-info-100);
  color: var(--color-info-600);
}

.icon-success {
  background: var(--color-success-100);
  color: var(--color-success-600);
}

.icon-warning {
  background: var(--color-warning-100);
  color: var(--color-warning-600);
}

.icon-danger {
  background: var(--color-danger-100);
  color: var(--color-danger-600);
}

.confirm-title {
  font-size: 1.125rem;
  font-weight: 600;
  color: var(--text-primary);
  margin: 0 0 8px;
}

.confirm-message {
  font-size: 0.9375rem;
  color: var(--text-secondary);
  margin: 0;
  line-height: 1.5;
}

.confirm-footer {
  display: flex;
  gap: 12px;
  width: 100%;
}

.btn {
  flex: 1;
  height: 44px;
  border: none;
  border-radius: 10px;
  font-size: 0.9375rem;
  font-weight: 500;
  cursor: pointer;
  transition: all var(--motion-duration-quick) var(--motion-ease-out);
}

.btn-cancel {
  background: var(--bg-tertiary);
  color: var(--text-secondary);
}

.btn-cancel:hover {
  background: var(--border-default);
  color: var(--text-primary);
}

.btn-info {
  background: var(--color-info-500);
  color: var(--text-inverse);
}

.btn-info:hover {
  background: var(--color-info-600);
}

.btn-success {
  background: var(--color-success-500);
  color: var(--text-inverse);
}

.btn-success:hover {
  background: var(--color-success-600);
}

.btn-warning {
  background: var(--color-warning-500);
  color: var(--text-inverse);
}

.btn-warning:hover {
  background: var(--color-warning-600);
}

.btn-danger {
  background: var(--color-danger-500);
  color: var(--text-inverse);
}

.btn-danger:hover {
  background: var(--color-danger-600);
}

</style>

<template>
    <div class="modal" role="dialog" aria-modal="true" aria-label="确认" @click.stop>
        <div class="modal-title-row">
            <div class="modal-title">{{ confirm.title }}</div>
        </div>

        <div class="confirm-message">{{ confirm.message }}</div>

        <div v-if="error" class="error">{{ error }}</div>

        <div class="modal-actions">
            <button class="btn btn-ghost" type="button" :disabled="confirm.pending" @click="emit('close')">
                取消
            </button>
            <button
                class="btn"
                :class="confirm.danger ? 'btn-danger' : 'btn-primary'"
                type="button"
                :disabled="confirm.pending"
                @click="emit('confirm')"
            >
                {{ confirm.pending ? '处理中…' : confirm.confirmText }}
            </button>
        </div>
    </div>
</template>

<script setup lang="ts">
import type { ConfirmModalState } from '../types';

defineProps<{
    confirm: ConfirmModalState;
    error: string | null;
}>();

const emit = defineEmits<{
    (e: 'close'): void;
    (e: 'confirm'): void;
}>();
</script>


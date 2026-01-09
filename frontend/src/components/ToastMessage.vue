<template>
    <div
        class="toast"
        :class="[`toast-${toast.kind}`, `toast-${toast.position ?? 'corner'}`]"
        role="status"
        @click="emit('dismiss')"
    >
        {{ toast.message }}
    </div>
</template>

<script setup lang="ts">
import type { ToastState } from '../types';

defineProps<{ toast: ToastState }>();

const emit = defineEmits<{
    (e: 'dismiss'): void;
}>();
</script>


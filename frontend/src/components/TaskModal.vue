<template>
    <div class="modal" role="dialog" aria-modal="true" aria-label="任务" @click.stop>
        <div class="modal-title-row">
            <div class="modal-title">{{ form.id ? '编辑任务' : '新增任务' }}</div>
            <button
                v-if="form.id"
                class="btn btn-danger"
                type="button"
                @click="emit('delete', { id: Number(form.id), title: String(form.title ?? '') })"
            >
                删除
            </button>
        </div>

        <form @submit.prevent="emit('submit', { ...form })">
            <label class="field">
                <div class="field-label">标题</div>
                <input
                    ref="titleEl"
                    class="input"
                    name="title"
                    type="text"
                    placeholder="请输入任务标题"
                    v-model="form.title"
                    @input="emit('clearError')"
                />
            </label>

            <label class="field">
                <div class="field-label">内容</div>
                <textarea
                    class="textarea"
                    name="content"
                    rows="4"
                    placeholder="可选"
                    v-model="form.content"
                    @input="emit('clearError')"
                ></textarea>
            </label>

            <label class="field">
                <div class="field-label">状态</div>
                <select class="select" name="status" v-model="form.status" @change="emit('clearError')">
                    <option v-for="st in statusValues" :key="st" :value="st">
                        {{ statusLabels[st] ?? st }}
                    </option>
                </select>
            </label>

            <div class="grid2">
                <label class="toggle">
                    <input class="checkbox" type="checkbox" v-model="form.important" />
                    <span>重要</span>
                </label>
                <label class="toggle">
                    <input class="checkbox" type="checkbox" v-model="form.urgent" />
                    <span>紧急</span>
                </label>
            </div>

            <div v-if="error" class="error">{{ error }}</div>

            <div class="modal-actions">
                <button class="btn btn-ghost" type="button" @click="emit('close')">取消</button>
                <button class="btn btn-primary" type="submit">保存</button>
            </div>
        </form>
    </div>
</template>

<script setup lang="ts">
import { nextTick, onMounted, reactive, ref, toRefs, watch } from 'vue';

import type { StatusValue, TaskModalState } from '../types';

const props = defineProps<{
    task: TaskModalState;
    error: string | null;
    statusValues: StatusValue[];
    statusLabels: Record<StatusValue, string>;
}>();

const { error, statusLabels, statusValues, task } = toRefs(props);

const emit = defineEmits<{
    (e: 'close'): void;
    (e: 'delete', payload: { id: number; title: string }): void;
    (e: 'submit', task: TaskModalState): void;
    (e: 'clearError'): void;
}>();

const form = reactive<TaskModalState>({ ...task.value });

watch(
    task,
    (next) => {
        Object.assign(form, next);
    },
    { deep: true },
);

const titleEl = ref<HTMLInputElement | null>(null);

onMounted(() => {
    nextTick(() => {
        titleEl.value?.focus?.();
    });
});
</script>

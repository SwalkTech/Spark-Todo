<template>
    <div v-if="matrixAreas" class="matrix" :style="{ gridTemplateAreas: matrixAreas }">
        <section
            v-for="q in visibleQuadrants"
            :key="q.key"
            class="quadrant"
            :style="{ gridArea: q.key }"
            :data-quadrant="q.key"
        >
            <div class="quadrant-header">
                <div class="quadrant-title">{{ q.title }}</div>
                <div class="quadrant-meta">
                    <span class="pill">{{ quadrantTasksMap[q.key].length }}</span>
                    <button
                        class="btn btn-ghost btn-icon"
                        type="button"
                        title="在此象限新增任务"
                        @click="emit('addTask', { important: q.important, urgent: q.urgent })"
                    >
                        +
                    </button>
                </div>
            </div>

            <div class="task-list" :class="viewMode">
                <template v-for="t in quadrantTasksMap[q.key]" :key="Number(t.id)">
                    <div v-if="viewMode === 'list'" class="task-row" :class="{ done: isDone(t) }">
                        <input
                            type="checkbox"
                            class="checkbox task-check"
                            :checked="isDone(t)"
                            aria-label="完成"
                            @change="onToggleTaskDone(t, $event)"
                        />
                        <button class="task-main" type="button" @click="emit('editTask', t)">
                            <div class="task-title">{{ t.title }}</div>
                            <div v-if="String(t.content ?? '').trim()" class="task-content">
                                {{ t.content }}
                            </div>
                        </button>
                    </div>

                    <button
                        v-else
                        class="task-card"
                        :class="{ done: isDone(t) }"
                        type="button"
                        @click="emit('editTask', t)"
                    >
                        <div class="task-title">{{ t.title }}</div>
                        <div v-if="String(t.content ?? '').trim()" class="task-content">
                            {{ t.content }}
                        </div>
                    </button>
                </template>
            </div>
        </section>
    </div>
</template>

<script setup lang="ts">
import { toRefs } from 'vue';

import type { todo } from '../../wailsjs/go/models';

import type { QuadrantKey, QuadrantPreset, ViewMode } from '../types';

type QuadrantMeta = { key: QuadrantKey; title: string; important: boolean; urgent: boolean };

const props = defineProps<{
    matrixAreas: string;
    visibleQuadrants: QuadrantMeta[];
    quadrantTasksMap: Record<QuadrantKey, todo.Task[]>;
    viewMode: ViewMode;
}>();

const { matrixAreas, quadrantTasksMap, viewMode, visibleQuadrants } = toRefs(props);

const emit = defineEmits<{
    (e: 'addTask', preset: QuadrantPreset): void;
    (e: 'editTask', task: todo.Task): void;
    (e: 'toggleTaskDone', payload: { task: todo.Task; checked: boolean }): void;
}>();

function isDone(task: todo.Task) {
    return String(task.status) === 'done';
}

function onToggleTaskDone(task: todo.Task, e: Event) {
    const el = e.target;
    if (!(el instanceof HTMLInputElement)) return;
    emit('toggleTaskDone', { task, checked: el.checked });
}
</script>

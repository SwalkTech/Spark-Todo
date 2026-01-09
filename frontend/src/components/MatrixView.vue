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
                    <!-- 列表视图 -->
                    <div v-if="viewMode === 'list'" class="task-item-wrapper">
                        <div class="task-row" :class="[getStatusClass(t), { done: isDone(t) }]">
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
                            <div class="task-actions">
                                <button
                                    v-if="hasSubTasks(t)"
                                    class="btn btn-ghost btn-icon btn-subtask-toggle"
                                    type="button"
                                    :title="isExpanded(t.id) ? '收起子任务' : '展开子任务'"
                                    @click.stop="toggleExpand(t.id)"
                                >
                                    {{ isExpanded(t.id) ? '▼' : '▶' }}
                                </button>
                                <button
                                    class="btn btn-ghost btn-icon btn-add-subtask"
                                    type="button"
                                    title="添加子任务"
                                    @click.stop="emit('addSubTask', t)"
                                >
                                    +
                                </button>
                                <span v-if="hasSubTasks(t)" class="subtask-count">
                                    {{ getSubTaskProgress(t) }}
                                </span>
                            </div>
                        </div>
                        <!-- 子任务列表 -->
                        <div v-if="hasSubTasks(t) && isExpanded(t.id)" class="subtask-list">
                            <div
                                v-for="st in t.subTasks"
                                :key="Number(st.id)"
                                class="subtask-row"
                                :class="[getStatusClass(st), { done: isDone(st) }]"
                            >
                                <input
                                    type="checkbox"
                                    class="checkbox task-check"
                                    :checked="isDone(st)"
                                    aria-label="完成"
                                    @change="onToggleTaskDone(st, $event)"
                                />
                                <button class="task-main" type="button" @click="emit('editTask', st)">
                                    <div class="task-title">{{ st.title }}</div>
                                    <div v-if="String(st.content ?? '').trim()" class="task-content">
                                        {{ st.content }}
                                    </div>
                                </button>
                            </div>
                        </div>
                    </div>

                    <!-- 卡片视图 -->
                    <div v-else class="task-item-wrapper">
                        <div class="task-card" :class="[getStatusClass(t), { done: isDone(t) }]">
                            <button class="task-card-main" type="button" @click="emit('editTask', t)">
                                <div class="task-title">{{ t.title }}</div>
                                <div v-if="String(t.content ?? '').trim()" class="task-content">
                                    {{ t.content }}
                                </div>
                            </button>
                            <div class="task-card-footer">
                                <div class="task-actions">
                                    <button
                                        v-if="hasSubTasks(t)"
                                        class="btn btn-ghost btn-icon btn-subtask-toggle"
                                        type="button"
                                        :title="isExpanded(t.id) ? '收起子任务' : '展开子任务'"
                                        @click.stop="toggleExpand(t.id)"
                                    >
                                        {{ isExpanded(t.id) ? '▼' : '▶' }}
                                    </button>
                                    <button
                                        class="btn btn-ghost btn-icon btn-add-subtask"
                                        type="button"
                                        title="添加子任务"
                                        @click.stop="emit('addSubTask', t)"
                                    >
                                        +
                                    </button>
                                    <span v-if="hasSubTasks(t)" class="subtask-count">
                                        {{ getSubTaskProgress(t) }}
                                    </span>
                                </div>
                            </div>
                        </div>
                        <!-- 子任务列表（卡片视图） -->
                        <div v-if="hasSubTasks(t) && isExpanded(t.id)" class="subtask-list subtask-list-cards">
                            <div
                                v-for="st in t.subTasks"
                                :key="Number(st.id)"
                                class="subtask-card"
                                :class="[getStatusClass(st), { done: isDone(st) }]"
                            >
                                <input
                                    type="checkbox"
                                    class="checkbox task-check"
                                    :checked="isDone(st)"
                                    aria-label="完成"
                                    @change="onToggleTaskDone(st, $event)"
                                />
                                <button class="task-main" type="button" @click="emit('editTask', st)">
                                    <div class="task-title">{{ st.title }}</div>
                                </button>
                            </div>
                        </div>
                    </div>
                </template>
            </div>
        </section>
    </div>
</template>

<script setup lang="ts">
import { ref, toRefs } from 'vue';

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
    (e: 'addSubTask', parentTask: todo.Task): void;
    (e: 'editTask', task: todo.Task): void;
    (e: 'toggleTaskDone', payload: { task: todo.Task; checked: boolean }): void;
}>();

// 存储展开状态的任务 ID
const expandedTasks = ref<Set<number>>(new Set());

function isDone(task: todo.Task) {
    return String(task.status) === 'done';
}

function getStatusClass(task: todo.Task): string {
    const status = String(task.status);
    return `status-${status}`;
}

function hasSubTasks(task: todo.Task): boolean {
    return Array.isArray(task.subTasks) && task.subTasks.length > 0;
}

function isExpanded(taskId: number): boolean {
    return expandedTasks.value.has(taskId);
}

function toggleExpand(taskId: number) {
    if (expandedTasks.value.has(taskId)) {
        expandedTasks.value.delete(taskId);
    } else {
        expandedTasks.value.add(taskId);
    }
}

function getSubTaskProgress(task: todo.Task): string {
    if (!task.subTasks || task.subTasks.length === 0) return '';
    const done = task.subTasks.filter((st) => String(st.status) === 'done').length;
    return `${done}/${task.subTasks.length}`;
}

function onToggleTaskDone(task: todo.Task, e: Event) {
    const el = e.target;
    if (!(el instanceof HTMLInputElement)) return;
    emit('toggleTaskDone', { task, checked: el.checked });
}
</script>

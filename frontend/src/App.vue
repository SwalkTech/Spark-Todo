<template>
    <div class="app-shell">
        <div v-if="loading" class="loading page-pad">加载中…</div>
        <div v-else-if="error" class="error-block page-pad">加载失败：{{ error }}</div>
        <MatrixView
            v-else-if="matrixAreas"
            :matrix-areas="matrixAreas"
            :visible-quadrants="visibleQuadrants"
            :quadrant-tasks-map="quadrantTasksMap"
            :view-mode="viewMode"
            @add-task="onAddTask"
            @edit-task="openTaskModal"
            @toggle-task-done="onToggleTaskDone"
        />
        <div v-else class="empty-state page-pad">暂无任务，点击右下角 + 新建</div>

        <button
            v-if="menuAllowed"
            class="fab fab-menu"
            type="button"
            aria-label="菜单"
            @click="toggleMenu"
        >
            ≡
        </button>
        <button
            class="fab fab-add"
            type="button"
            title="新增任务"
            aria-label="新增任务"
            @click="onAddTask(lastPreset)"
        >
            +
        </button>

        <DrawerMenu
            v-if="menuAllowed && (drawerOpen || drawerClosing)"
            :phase="drawerOpen ? 'open' : 'closing'"
            :settings="settings"
            :view-mode="viewMode"
            :theme="currentTheme"
            @close="closeMenu"
            @closed="onDrawerClosed"
            @set-view-mode="setViewMode"
            @toggle-hide-done="toggleHideDone"
            @toggle-always-on-top="toggleAlwaysOnTop"
            @toggle-theme="toggleTheme"
            @toggle-concise-mode="toggleConciseMode"
            @check-updates="checkForUpdates(true)"
            @quit="quitApp"
        />

        <div v-if="modal" class="modal-root">
            <div class="modal-backdrop" @click="closeModal"></div>
            <TaskModal
                v-if="modal.kind === 'task'"
                :task="modal"
                :error="modalError"
                :status-values="statusValues"
                :status-labels="statusLabels"
                @close="closeModal"
                @clear-error="modalError = null"
                @submit="submitTask"
                @delete="onDeleteTaskInModal"
            />
            <ConfirmModal
                v-else-if="modal.kind === 'confirm'"
                :confirm="modal"
                :error="modalError"
                @close="closeModal"
                @confirm="confirmOk"
            />
            <UpdateModal
                v-else-if="modal.kind === 'update'"
                :update-info="modal.updateInfo"
                :pending="modal.pending"
                :description="updateDescription"
                :error="modalError"
                @close="closeModal"
                @view-release="viewRelease"
                @download="downloadUpdate"
            />
        </div>

        <ToastMessage v-if="toast" :toast="toast" @dismiss="dismissToast" />
    </div>
</template>

<script setup lang="ts">
import { computed, onBeforeUnmount, onMounted, ref } from 'vue';

import {
    CheckUpdate,
    DeleteTask,
    GetBoard,
    OpenURL,
    Quit,
    Restart,
    SetAlwaysOnTop,
    SetConciseMode,
    SetHideDone,
    SetTheme,
    SetViewMode,
    ShowWaterReminder,
    UpsertTask,
} from '../wailsjs/go/main/App';

import type { todo } from '../wailsjs/go/models';

import { animateThemeTransition, getCurrentTheme, normalizeTheme, persistTheme, setDocumentTheme } from './theme';

import ConfirmModal from './components/ConfirmModal.vue';
import DrawerMenu from './components/DrawerMenu.vue';
import MatrixView from './components/MatrixView.vue';
import TaskModal from './components/TaskModal.vue';
import ToastMessage from './components/ToastMessage.vue';
import UpdateModal from './components/UpdateModal.vue';

import type { ModalState, QuadrantKey, QuadrantPreset, StatusValue, TaskModalState, ToastState, ViewMode } from './types';

const statusLabels: Record<StatusValue, string> = {
    todo: '待办',
    doing: '进行中',
    done: '已完成',
};

const statusValues: StatusValue[] = ['todo', 'doing', 'done'];

const quadrants = [
    { key: 'iu', title: '重要且紧急', important: true, urgent: true },
    { key: 'in', title: '重要不紧急', important: true, urgent: false },
    { key: 'nu', title: '不重要但紧急', important: false, urgent: true },
    { key: 'nn', title: '不重要不紧急', important: false, urgent: false },
] as const satisfies ReadonlyArray<{
    key: QuadrantKey;
    title: string;
    important: boolean;
    urgent: boolean;
}>;

const MENU_MIN_SIZE_PX = 500;
const WATER_REMINDER_INTERVAL_MS = 2.5 * 60 * 60 * 1000;

declare global {
    interface Window {
        __sparkTodoWaterReminderStarted?: boolean;
    }
}

const board = ref<todo.Board | null>(null);
const loading = ref(false);
const error = ref<string | null>(null);

const drawerOpen = ref(false);
const drawerClosing = ref(false);

const menuAllowed = ref(true);

const lastPreset = ref<QuadrantPreset>({ important: false, urgent: false });

const modal = ref<ModalState>(null);
const modalError = ref<string | null>(null);

const toast = ref<ToastState | null>(null);
let toastTimer: number | null = null;

let waterReminderTimer: number | null = null;
let updateCheckTimer: number | null = null;

const defaultSettings: todo.Settings = {
    hideDone: false,
    alwaysOnTop: true,
    viewMode: 'cards',
    conciseMode: false,
    theme: 'light',
} as any;

const settings = computed<todo.Settings>(() => board.value?.settings ?? defaultSettings);

const viewMode = computed<ViewMode>(() => normalizeViewMode(settings.value.viewMode));
const currentTheme = computed(() => normalizeTheme((settings.value as any).theme));

const tasks = computed<todo.Task[]>(() => {
    const all = board.value?.tasks ?? [];
    return settings.value.hideDone ? all.filter((t) => String(t.status) !== 'done') : all;
});

const quadrantTasksMap = computed<Record<QuadrantKey, todo.Task[]>>(() => {
    const map: Record<QuadrantKey, todo.Task[]> = { iu: [], in: [], nu: [], nn: [] };
    for (const t of tasks.value) {
        map[quadrantKey(t)].push(t);
    }
    return map;
});

const visibleQuadrants = computed(() =>
    quadrants.filter((q) => (quadrantTasksMap.value[q.key] ?? []).length > 0),
);

const matrixAreas = computed(() => {
    const keys = new Set<QuadrantKey>(visibleQuadrants.value.map((q) => q.key));
    return computeMatrixTemplateAreas(keys);
});

const updateDescription = computed(() => {
    const release = modal.value?.kind === 'update' ? modal.value.updateInfo.latestRelease : undefined;
    let description = String(release?.description ?? '暂无更新说明').trim();
    if (description.length > 500) description = description.substring(0, 500) + '...';
    return description;
});

function normalizeViewMode(mode: unknown): ViewMode {
    return mode === 'list' ? 'list' : 'cards';
}

function isMenuAllowed() {
    return window.innerWidth >= MENU_MIN_SIZE_PX && window.innerHeight >= MENU_MIN_SIZE_PX;
}

function updateMenuAllowed() {
    menuAllowed.value = isMenuAllowed();
    if (!menuAllowed.value) {
        drawerOpen.value = false;
        drawerClosing.value = false;
    }
}

function setDrawerOpen(nextOpen: boolean) {
    if (nextOpen) {
        drawerOpen.value = true;
        drawerClosing.value = false;
        return;
    }

    if (!menuAllowed.value) {
        drawerOpen.value = false;
        drawerClosing.value = false;
        return;
    }

    if (!drawerOpen.value && !drawerClosing.value) return;

    drawerOpen.value = false;

    const prefersReducedMotion = window.matchMedia('(prefers-reduced-motion: reduce)').matches;
    if (prefersReducedMotion) {
        drawerClosing.value = false;
        return;
    }

    drawerClosing.value = true;
}

function toggleMenu() {
    if (drawerClosing.value) return;
    if (!menuAllowed.value) {
        setDrawerOpen(false);
        return;
    }
    setDrawerOpen(!drawerOpen.value);
}

function closeMenu() {
    setDrawerOpen(false);
}

function onDrawerClosed() {
    drawerClosing.value = false;
}

function formatError(err: unknown): string {
    if (!err) return '未知错误';
    if (typeof err === 'string') return err;
    if (err && typeof err === 'object' && 'message' in err && typeof (err as any).message === 'string') {
        return (err as any).message;
    }
    return String(err);
}

function showToast(
    message: string,
    kind: 'error' | 'success' = 'error',
    timeoutMs = 2500,
    position: 'corner' | 'center' = 'center',
): void {
    const text = String(message ?? '').trim();
    if (!text) return;

    toast.value = {
        kind: kind === 'success' ? 'success' : 'error',
        message: text,
        position: position === 'corner' ? 'corner' : 'center',
    };

    if (toastTimer) clearTimeout(toastTimer);
    toastTimer = window.setTimeout(() => {
        toast.value = null;
        toastTimer = null;
    }, timeoutMs);
}

function dismissToast() {
    toast.value = null;
    if (toastTimer) clearTimeout(toastTimer);
    toastTimer = null;
}

function quadrantKey(task: todo.Task): QuadrantKey {
    const important = !!(task as any).important;
    const urgent = !!(task as any).urgent;
    if (important && urgent) return 'iu';
    if (important && !urgent) return 'in';
    if (!important && urgent) return 'nu';
    return 'nn';
}

function computeMatrixTemplateAreas(keys: Set<QuadrantKey> | null | undefined): string {
    if (!keys || keys.size <= 0) return '';

    const hasIU = keys.has('iu');
    const hasIN = keys.has('in');
    const hasNU = keys.has('nu');
    const hasNN = keys.has('nn');

    const count = keys.size;

    if (count === 1) {
        const only = keys.values().next().value as QuadrantKey;
        return `'${only} ${only}' '${only} ${only}'`;
    }

    if (count === 3) {
        if (!hasIU) return `'in in' 'nu nn'`;
        if (!hasIN) return `'iu iu' 'nu nn'`;
        if (!hasNU) return `'iu in' 'nn nn'`;
        return `'iu in' 'nu nu'`;
    }

    if (count === 2) {
        const preferStack = window.innerWidth <= window.innerHeight;
        if (hasIU && hasIN) return preferStack ? `'iu iu' 'in in'` : `'iu in' 'iu in'`;
        if (hasNU && hasNN) return preferStack ? `'nu nu' 'nn nn'` : `'nu nn' 'nu nn'`;
        if (hasIU && hasNU) return `'iu iu' 'nu nu'`;
        if (hasIN && hasNN) return `'in in' 'nn nn'`;
        if (hasIU && hasNN) return `'iu iu' 'nn nn'`;
        if (hasIN && hasNU) return `'in in' 'nu nu'`;
    }

    return `'iu in' 'nu nn'`;
}

function getDefaultGroupId() {
    return Number(board.value?.groups?.[0]?.id ?? 0);
}

function normalizeStatusValue(value: unknown): StatusValue {
    const v = String(value ?? '');
    return v === 'doing' || v === 'done' ? (v as StatusValue) : 'todo';
}

function openTaskModal(task: todo.Task) {
    const defaultGroupId = getDefaultGroupId();
    if (!defaultGroupId && !task?.groupId) {
        showToast('初始化未完成，请稍后重试');
        return;
    }

    modalError.value = null;
    modal.value = {
        kind: 'task',
        id: Number(task?.id ?? 0),
        groupId: Number(task?.groupId ?? defaultGroupId),
        title: String((task as any)?.title ?? ''),
        content: String((task as any)?.content ?? ''),
        status: normalizeStatusValue((task as any)?.status),
        important: Boolean((task as any)?.important ?? false),
        urgent: Boolean((task as any)?.urgent ?? false),
    };
}

function onAddTask(preset?: QuadrantPreset) {
    const defaultGroupId = getDefaultGroupId();
    if (!defaultGroupId) {
        showToast('初始化未完成，请稍后重试');
        return;
    }

    if (preset) lastPreset.value = preset;

    modalError.value = null;
    modal.value = {
        kind: 'task',
        id: 0,
        groupId: defaultGroupId,
        title: '',
        content: '',
        status: 'todo',
        important: Boolean(preset?.important ?? lastPreset.value.important ?? false),
        urgent: Boolean(preset?.urgent ?? lastPreset.value.urgent ?? false),
    };
}

function closeModal() {
    modal.value = null;
    modalError.value = null;
}

function onDeleteTaskInModal(payload: { id: number; title: string }) {
    const taskId = Number(payload.id);
    if (!Number.isFinite(taskId) || taskId <= 0) return;

    modalError.value = null;
    modal.value = {
        kind: 'confirm',
        title: '删除任务',
        message: payload.title ? `确定删除任务「${payload.title}」？` : '确定删除该任务？',
        targetType: 'task',
        targetId: taskId,
        confirmText: '删除',
        danger: true,
        pending: false,
    };
}

async function confirmOk() {
    const m = modal.value;
    if (!m || m.kind !== 'confirm') return;

    modalError.value = null;
    modal.value = { ...m, pending: true };

    try {
        await DeleteTask(Number(m.targetId));
        await refresh();
        closeModal();
    } catch (err) {
        modalError.value = formatError(err);
        modal.value = { ...m, pending: false };
    }
}

function validateTaskModal(m: TaskModalState) {
    const groupId = Number(m.groupId ?? getDefaultGroupId());
    if (!Number.isFinite(groupId) || groupId <= 0) throw new Error('初始化未完成，请稍后重试');

    const title = String(m.title ?? '').trim();
    if (!title) throw new Error('任务标题不能为空');
    if (Array.from(title).length > 200) throw new Error('任务标题过长（最多 200 字）');

    const content = String(m.content ?? '').trim();
    if (Array.from(content).length > 1000) throw new Error('任务内容过长（最多 1000 字）');

    return { groupId, title, content };
}

async function submitTask(m: TaskModalState) {
    modalError.value = null;

    try {
        const { groupId, title, content } = validateTaskModal(m);
        const task: todo.Task = {
            id: Number(m.id ?? 0),
            groupId,
            status: String(m.status ?? 'todo'),
            title,
            content,
            important: !!m.important,
            urgent: !!m.urgent,
            createdAt: 0,
            updatedAt: 0,
        } as any;

        await UpsertTask(task);
        closeModal();
        await refresh();
        showToast('已保存', 'success');
    } catch (err) {
        modalError.value = formatError(err);
    }
}

async function onToggleTaskDone(payload: { task: todo.Task; checked: boolean }) {
    const nextStatus = payload.checked ? 'done' : 'todo';
    try {
        await UpsertTask({ ...payload.task, status: nextStatus } as any);
        await refresh();
    } catch (err) {
        showToast(formatError(err));
    }
}

async function setViewMode(mode: ViewMode) {
    try {
        const next = await SetViewMode(mode);
        if (board.value) board.value.settings = next;
    } catch (err) {
        showToast(formatError(err));
    }
}

async function toggleHideDone(checked: boolean) {
    try {
        const next = await SetHideDone(checked);
        if (board.value) board.value.settings = next;
    } catch (err) {
        showToast(formatError(err));
    }
}

async function toggleAlwaysOnTop(checked: boolean) {
    try {
        const next = await SetAlwaysOnTop(checked);
        if (board.value) board.value.settings = next;
    } catch (err) {
        showToast(formatError(err));
    }
}

async function toggleConciseMode(checked: boolean) {
    try {
        const next = await SetConciseMode(checked);
        if (board.value) board.value.settings = next;
        showToast('简洁模式已保存，正在重启应用...', 'success', 2000);
        window.setTimeout(async () => {
            try {
                await Restart();
            } catch (err) {
                console.error('重启失败:', err);
                showToast('自动重启失败，请手动重启应用', 'error', 5000);
            }
        }, 1000);
    } catch (err) {
        showToast(formatError(err));
    }
}

async function toggleTheme(payload: { checked: boolean; origin: { x: number; y: number } }) {
    const nextTheme = payload.checked ? 'dark' : 'light';
    const prevTheme = getCurrentTheme();

    try {
        const next = await SetTheme(nextTheme);
        if (board.value) board.value.settings = next;
        persistTheme(nextTheme);
        await animateThemeTransition(nextTheme, payload.origin);
    } catch (err) {
        persistTheme(prevTheme);
        setDocumentTheme(prevTheme);
        showToast(formatError(err));
    }
}

function syncThemeFromSettings(next: todo.Settings | null | undefined) {
    if (!next) return;
    const theme = normalizeTheme((next as any).theme);
    setDocumentTheme(theme);
    persistTheme(theme);
}

async function refresh() {
    loading.value = true;
    error.value = null;
    try {
        board.value = await GetBoard();
        error.value = null;
        syncThemeFromSettings(board.value?.settings);
    } catch (err) {
        error.value = formatError(err);
    } finally {
        loading.value = false;
    }
}

async function checkForUpdates(showNoUpdateMessage = false) {
    try {
        const result = await CheckUpdate();
        if (result.hasUpdate && result.latestRelease) {
            modalError.value = null;
            modal.value = { kind: 'update', updateInfo: result, pending: false };
        } else if (showNoUpdateMessage) {
            showToast('当前已是最新版本', 'success');
        }
    } catch (err) {
        console.error('检查更新失败:', err);
        if (showNoUpdateMessage) showToast('检查更新失败，请稍后重试', 'error');
    }
}

async function viewRelease() {
    const m = modal.value;
    if (!m || m.kind !== 'update') return;
    const pageUrl = m.updateInfo.latestRelease?.pageUrl;
    if (!pageUrl) {
        showToast('未找到详情页面链接', 'error', 2500, 'center');
        return;
    }
    try {
        await OpenURL(pageUrl);
        closeModal();
    } catch (err) {
        showToast(formatError(err));
    }
}

async function downloadUpdate() {
    const m = modal.value;
    if (!m || m.kind !== 'update') return;
    const downloadUrl = m.updateInfo.latestRelease?.downloadUrl;
    if (!downloadUrl) {
        showToast('未找到下载链接', 'error', 2500, 'center');
        return;
    }
    try {
        await OpenURL(downloadUrl);
        showToast('已在浏览器中打开下载页面', 'success');
        closeModal();
    } catch (err) {
        showToast(formatError(err));
    }
}

async function quitApp() {
    await Quit();
}

function startWaterReminder(immediate = true) {
    if (waterReminderTimer) return;
    if (window.__sparkTodoWaterReminderStarted) return;
    window.__sparkTodoWaterReminderStarted = true;

    const trigger = () => {
        ShowWaterReminder().catch((err: any) => {
            console.error(err);
            showToast('喝水小提醒：该喝水了', 'success', 5000, 'center');
        });
    };

    if (immediate) trigger();
    waterReminderTimer = window.setInterval(trigger, WATER_REMINDER_INTERVAL_MS);
}

function onKeydown(e: KeyboardEvent) {
    if (e.key !== 'Escape') return;
    if (modal.value) {
        e.preventDefault();
        closeModal();
        return;
    }
    if (drawerOpen.value || drawerClosing.value) {
        e.preventDefault();
        closeMenu();
    }
}

onMounted(() => {
    updateMenuAllowed();
    window.addEventListener('resize', updateMenuAllowed);
    document.addEventListener('keydown', onKeydown);

    refresh();
    startWaterReminder(true);

    updateCheckTimer = window.setTimeout(() => {
        checkForUpdates(false);
    }, 3000);
});

onBeforeUnmount(() => {
    window.removeEventListener('resize', updateMenuAllowed);
    document.removeEventListener('keydown', onKeydown);

    if (toastTimer) clearTimeout(toastTimer);
    toastTimer = null;

    if (updateCheckTimer) clearTimeout(updateCheckTimer);
    updateCheckTimer = null;

    if (waterReminderTimer) clearInterval(waterReminderTimer);
    waterReminderTimer = null;
    window.__sparkTodoWaterReminderStarted = false;
});
</script>

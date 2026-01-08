import './style.css';
import './app.css';

/**
 * 前端入口（Vanilla JS + Wails）。
 *
 * 核心思路：
 * - 数据层：通过 Wails 生成的桥接函数调用 Go 后端（见 `app.go`），后端再读写 SQLite。
 * - 视图层：不依赖框架，`render()` 直接把 state 渲染成 HTML 字符串并写入 `#app`。
 * - 交互层：使用事件委托（click/change/submit 统一绑在 `#app` 上），通过 `data-action` 分发操作。
 *
 * 注意：因为使用 `innerHTML` 渲染，任何用户输入都必须经过 `escapeHtml()`，避免 DOM 注入。
 */

import {
    CheckUpdate,
    DeleteTask,
    GetBoard,
    GetVersion,
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

import type {todo} from '../wailsjs/go/models';
import type {version} from '../wailsjs/go/models';

type StatusValue = 'todo' | 'doing' | 'done';
type ViewMode = 'list' | 'cards';
type ToastKind = 'error' | 'success';
type ToastPosition = 'corner' | 'center';
type QuadrantKey = 'iu' | 'in' | 'nu' | 'nn';
type Theme = 'light' | 'dark';

type QuadrantPreset = { important: boolean; urgent: boolean };

type TaskModal = {
    kind: 'task';
    id: number;
    groupId: number;
    title: string;
    content: string;
    status: StatusValue;
    important: boolean;
    urgent: boolean;
};

type ConfirmModal = {
    kind: 'confirm';
    title: string;
    message: string;
    targetType: 'task';
    targetId: number;
    confirmText: string;
    danger: boolean;
    pending: boolean;
};

type UpdateModal = {
    kind: 'update';
    updateInfo: version.UpdateCheckResult;
    pending: boolean;
};

type ModalState = TaskModal | ConfirmModal | UpdateModal | null;

type ToastState = {
    kind: ToastKind;
    message: string;
    position: ToastPosition;
};

type State = {
    board: todo.Board | null;
    loading: boolean;
    error: string | null;
    drawerOpen: boolean;
    lastPreset: QuadrantPreset;
    modal: ModalState;
    modalError: string | null;
    toast: ToastState | null;
};

declare global {
    interface Window {
        __sparkTodoWaterReminderStarted?: boolean;
    }
}

// 状态展示文案（用于下拉框/渲染标题）。
const statusLabels: Record<StatusValue, string> = {
    todo: '待办',
    doing: '进行中',
    done: '已完成',
};

// 允许的状态值集合：用于前端校验与渲染选项（避免散落硬编码）。
const statusValues: StatusValue[] = ['todo', 'doing', 'done'];

function isStatusValue(value: string): value is StatusValue {
    return value === 'todo' || value === 'doing' || value === 'done';
}

function normalizeStatusValue(value: unknown): StatusValue {
    const v = String(value ?? '');
    return isStatusValue(v) ? v : 'todo';
}

// 四象限定义（艾森豪威尔矩阵）：important/urgent 两个维度决定象限。
const quadrants = [
    {key: 'iu', title: '重要且紧急', important: true, urgent: true},
    {key: 'in', title: '重要不紧急', important: true, urgent: false},
    {key: 'nu', title: '不重要但紧急', important: false, urgent: true},
    {key: 'nn', title: '不重要不紧急', important: false, urgent: false},
] as const satisfies ReadonlyArray<{
    key: QuadrantKey;
    title: string;
    important: boolean;
    urgent: boolean;
}>;

// 小窗口下隐藏菜单（让主要空间留给任务本身）。
const MENU_MIN_SIZE_PX = 500;

// “喝水提醒”间隔：启动后触发一次（后端会基于持久化记录做 1 小时去重），之后每 2.5 小时触发一次。
const WATER_REMINDER_INTERVAL_MS = 2.5 * 60 * 60 * 1000;
const THEME_STORAGE_KEY = 'sparkTodoTheme';

const appEl = (() => {
    const el = document.querySelector<HTMLElement>('#app');
    if (!el) throw new Error('Missing #app element');
    return el;
})();

const state: State = {
    board: null,
    loading: false,
    error: null,
    drawerOpen: false,
    lastPreset: {important: false, urgent: false},
    modal: null,
    modalError: null,
    toast: null,
};

let toastTimer: number | null = null;
let waterReminderTimer: number | null = null;
let resizeRaf: number | null = null;

function normalizeTheme(value: unknown): Theme {
    const v = String(value ?? '').trim().toLowerCase();
    return v === 'dark' ? 'dark' : 'light';
}

function getCurrentTheme(): Theme {
    return normalizeTheme(document.documentElement.getAttribute('data-theme'));
}

function setDocumentTheme(theme: Theme) {
    document.documentElement.setAttribute('data-theme', theme);
    document.documentElement.style.colorScheme = theme === 'dark' ? 'dark' : 'light';
}

function loadStoredTheme(): Theme | null {
    try {
        const raw = localStorage.getItem(THEME_STORAGE_KEY);
        if (!raw) return null;
        return normalizeTheme(raw);
    } catch {
        return null;
    }
}

function persistTheme(theme: Theme) {
    try {
        localStorage.setItem(THEME_STORAGE_KEY, theme);
    } catch {
        // ignore
    }
}

function syncThemeFromSettings(settings: todo.Settings | null | undefined) {
    if (!settings) return;
    const theme = normalizeTheme((settings as any).theme);
    setDocumentTheme(theme);
    persistTheme(theme);
}

async function animateThemeTransition(nextTheme: Theme, origin: {x: number; y: number}) {
    const prefersReducedMotion = window.matchMedia('(prefers-reduced-motion: reduce)').matches;
    const doc: any = document as any;
    if (prefersReducedMotion || typeof doc.startViewTransition !== 'function') {
        setDocumentTheme(nextTheme);
        return;
    }

    const endRadius = Math.hypot(
        Math.max(origin.x, window.innerWidth - origin.x),
        Math.max(origin.y, window.innerHeight - origin.y),
    );

    const transition = doc.startViewTransition(() => {
        setDocumentTheme(nextTheme);
    });

    try {
        await transition.ready;
    } catch {
        return;
    }

    const anim = (document.documentElement as any).animate(
        {
            clipPath: [
                `circle(0px at ${origin.x}px ${origin.y}px)`,
                `circle(${endRadius}px at ${origin.x}px ${origin.y}px)`,
            ],
        },
        {
            duration: 450,
            easing: 'cubic-bezier(0.5, 0, 1, 1)',
            pseudoElement: '::view-transition-new(root)',
        },
    );
    await anim.finished.catch(() => {
        // ignore
    });
}

// escapeHtml 用于把用户输入安全地插入到 innerHTML 中，避免 DOM 注入/XSS。
function escapeHtml(s: unknown): string {
    return String(s)
        .replaceAll('&', '&amp;')
        .replaceAll('<', '&lt;')
        .replaceAll('>', '&gt;')
        .replaceAll('"', '&quot;')
        .replaceAll("'", '&#039;');
}

function formatError(err: unknown): string {
    if (!err) return '未知错误';
    if (typeof err === 'string') return err;
    if (err && typeof err === 'object' && 'message' in err && typeof (err as any).message === 'string') {
        return (err as any).message;
    }
    return String(err);
}

// showToast 展示一个轻量提示：
// - kind=success|error 控制样式
// - 到期自动消失（也可点击手动关闭）
function showToast(
    message: unknown,
    kind: ToastKind = 'success',
    timeoutMs = 2500,
    position: ToastPosition = 'center',
): void {
    const text = String(message ?? '').trim();
    if (!text) return;

    const safeKind: ToastKind = kind === 'success' ? 'success' : 'error';
    const safePosition: ToastPosition = position === 'center' ? 'center' : 'corner';
    state.toast = {kind: safeKind, message: text, position: safePosition};
    render();

    if (toastTimer) clearTimeout(toastTimer);
    toastTimer = setTimeout(() => {
        state.toast = null;
        render();
        toastTimer = null;
    }, timeoutMs);
}

// normalizeViewMode 将任意输入归一化为受支持的视图模式。
function normalizeViewMode(mode: unknown): ViewMode {
    return mode === 'list' ? 'list' : 'cards';
}

// isMenuAllowed 用于在小窗口下隐藏菜单入口，避免 UI 过于拥挤。
function isMenuAllowed() {
    return window.innerWidth >= MENU_MIN_SIZE_PX && window.innerHeight >= MENU_MIN_SIZE_PX;
}

// quadrantKey 根据任务的 important/urgent 返回其所在象限 key。
function quadrantKey(task: todo.Task): QuadrantKey {
    const important = !!task.important;
    const urgent = !!task.urgent;
    if (important && urgent) return 'iu';
    if (important && !urgent) return 'in';
    if (!important && urgent) return 'nu';
    return 'nn';
}

// computeMatrixTemplateAreas 生成 CSS grid 的 template-areas。
//
// 本应用会隐藏“空象限”。为了避免出现网格空洞，这里根据当前可见象限组合动态合并区域，
// 让剩余象限尽可能铺满显示空间。
function computeMatrixTemplateAreas(keys: Set<QuadrantKey> | null | undefined): string {
    if (!keys || keys.size <= 0) return '';

    const hasIU = keys.has('iu');
    const hasIN = keys.has('in');
    const hasNU = keys.has('nu');
    const hasNN = keys.has('nn');

    const count = keys.size;
    if (count === 4) return `'iu in' 'nu nn'`;

    if (count === 1) {
        const k = Array.from(keys)[0];
        return `'${k} ${k}' '${k} ${k}'`;
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

// startWaterReminder 启动喝水提醒。
//
// `window.__sparkTodoWaterReminderStarted` 用于防止开发时热更新/重复初始化导致多重定时器。
function startWaterReminder() {
    if (waterReminderTimer) return;
    if (window.__sparkTodoWaterReminderStarted) return;
    window.__sparkTodoWaterReminderStarted = true;

    const trigger = () => {
        ShowWaterReminder().catch((err: any) => {
            console.error(err);
            showToast('喝水小提醒：该喝水了', 'success', 5000, 'center');
        });
    };

    trigger();

    waterReminderTimer = setInterval(() => {
        trigger();
    }, WATER_REMINDER_INTERVAL_MS);
}

// 默认组：后端会确保至少有一个默认组，因此这里取 groups[0] 作为“兜底”组。
function getDefaultGroupId() {
    return state.board?.groups?.[0]?.id ?? 0;
}

// 从当前 board 中按 id 查找任务（用于编辑/删除/勾选完成）。
function getTaskById(taskId: number): todo.Task | null {
    const id = Number(taskId);
    return state.board?.tasks?.find((t) => Number(t.id) === id) ?? null;
}

// 打开“任务编辑/新增”弹窗；preset 用于按象限快速创建（自动带 important/urgent）。
function openTaskModal(task: todo.Task | null, preset?: QuadrantPreset): void {
    const defaultGroupId = getDefaultGroupId();
    if (!defaultGroupId && !task?.groupId) {
        showToast('初始化未完成，请稍后重试');
        return;
    }

    state.modalError = null;
    state.modal = {
        kind: 'task',
        id: task?.id ?? 0,
        groupId: task?.groupId ?? defaultGroupId,
        title: task?.title ?? '',
        content: task?.content ?? '',
        status: normalizeStatusValue(task?.status),
        important: task?.important ?? preset?.important ?? false,
        urgent: task?.urgent ?? preset?.urgent ?? false,
    };
    render();
    queueMicrotask(() => {
        (document.querySelector('[data-focus="task-title"]') as HTMLElement | null)?.focus();
    });
}

// 打开“确认”弹窗（目前用于删除任务）。
function openConfirmModal({
                              title,
                              message,
                              targetType,
                              targetId,
                              confirmText,
                              danger,
                          }: {
    title?: string;
    message?: string;
    targetType: 'task';
    targetId: number;
    confirmText?: string;
    danger?: boolean;
}): void {
    state.modalError = null;
    state.modal = {
        kind: 'confirm',
        title: title ?? '确认',
        message: message ?? '',
        targetType,
        targetId,
        confirmText: confirmText ?? '确定',
        danger: !!danger,
        pending: false,
    };
    render();
    queueMicrotask(() => {
        (document.querySelector('[data-focus="confirm-ok"]') as HTMLElement | null)?.focus();
    });
}

// 关闭任意弹窗并清理错误状态。
function closeModal() {
    state.modalError = null;
    state.modal = null;
    render();
}

// refresh 从后端拉取最新 board，并驱动 UI 进入 loading/failed/ready 三态。
async function refresh() {
    state.loading = true;
    state.error = null;
    render();
    try {
        state.board = await GetBoard();
        state.error = null;
        syncThemeFromSettings(state.board?.settings);
    } catch (err) {
        state.error = formatError(err);
    } finally {
        state.loading = false;
        render();
    }
}

// renderTaskItem 将单个任务渲染为列表行或卡片。
function renderTaskItem(task: todo.Task, viewMode: ViewMode): string {
    const taskId = Number(task.id);
    const done = String(task.status) === 'done';
    const content = String(task.content ?? '').trim();

    if (viewMode === 'list') {
        return `
      <div class="task-row ${done ? 'done' : ''}">
        <input type="checkbox" class="checkbox task-check" data-action="toggle-task-done" data-task-id="${taskId}" ${
            done ? 'checked' : ''
        } aria-label="完成" />
        <button class="task-main" data-action="edit-task" data-task-id="${taskId}">
          <div class="task-title">${escapeHtml(task.title)}</div>
          ${content ? `<div class="task-content">${escapeHtml(content)}</div>` : ''}
        </button>
      </div>
    `;
    }

    return `
    <button class="task-card ${done ? 'done' : ''}" data-action="edit-task" data-task-id="${taskId}">
      <div class="task-title">${escapeHtml(task.title)}</div>
      ${content ? `<div class="task-content">${escapeHtml(content)}</div>` : ''}
    </button>
   `;
}

// renderDrawer 渲染左上角菜单抽屉（视图切换、隐藏已完成、置顶开关）。
function renderDrawer(settings: todo.Settings): string {
    const viewMode = normalizeViewMode(settings.viewMode);
    const theme = normalizeTheme((settings as any).theme);
    return `
    <div class="drawer-root">
      <div class="drawer-backdrop" data-action="toggle-menu"></div>
      <aside class="drawer" role="dialog" aria-label="菜单" aria-modal="true">
        <div class="drawer-title">菜单</div>

        <div class="drawer-section">
          <div class="drawer-section-title">视图</div>
          <div class="seg">
            <button class="btn ${viewMode === 'cards' ? 'btn-primary' : ''}" data-action="set-view-mode" data-view-mode="cards">卡片</button>
            <button class="btn ${viewMode === 'list' ? 'btn-primary' : ''}" data-action="set-view-mode" data-view-mode="list">列表</button>
          </div>
        </div>

        <div class="drawer-section">
          <label class="toggle">
            <input type="checkbox" class="checkbox" data-action="toggle-hide-done" ${settings.hideDone ? 'checked' : ''} />
            <span>隐藏已完成</span>
          </label>
          <label class="toggle">
            <input type="checkbox" class="checkbox" data-action="toggle-always-on-top" ${settings.alwaysOnTop ? 'checked' : ''} />
            <span>置顶悬浮</span>
          </label>
          <label class="toggle">
            <input type="checkbox" class="checkbox" data-action="toggle-theme" ${theme === 'dark' ? 'checked' : ''} />
            <span>夜间模式</span>
          </label>
          <label class="toggle">
            <input type="checkbox" class="checkbox" data-action="toggle-concise-mode" ${settings.conciseMode ? 'checked' : ''} />
            <span>简洁模式</span>
          </label>
        </div>

        <div class="drawer-section">
          <button class="btn btn-ghost" data-action="check-update">检查更新</button>
          <button class="btn btn-ghost" data-action="quit-app">退出应用</button>
          <button class="btn btn-ghost" data-action="toggle-menu">关闭菜单</button>
        </div>
      </aside>
    </div>
   `;
}

// render 是整个 UI 的唯一渲染入口：根据 state 派生出“可见象限/任务/弹窗/Toast”等，
// 最终一次性写入 `#app`。
function render() {
    const menuAllowed = isMenuAllowed();
    if (!menuAllowed) state.drawerOpen = false;

    const board = state.board;
    const settings: todo.Settings = board?.settings ?? {
        hideDone: false,
        alwaysOnTop: true,
        viewMode: 'cards',
        conciseMode: false,
        theme: 'light',
    };
    const viewMode = normalizeViewMode(settings.viewMode);
    const hideDone = settings.hideDone;
    const conciseMode = settings.conciseMode;


    const allTasks: todo.Task[] = board?.tasks ?? [];
    // 隐藏已完成：只隐藏 done 任务本身，列/象限布局仍按“是否有任务”来决定渲染与铺满。
    const tasks = hideDone ? allTasks.filter((t) => String(t.status) !== 'done') : allTasks;

    // 预构建象限索引：避免在渲染时对每个象限重复 filter（任务多时更明显）。
    const quadrantIndex = new Map<QuadrantKey, todo.Task[]>();
    for (const q of quadrants) quadrantIndex.set(q.key, []);
    for (const t of tasks) {
        const key = quadrantKey(t);
        quadrantIndex.get(key)?.push(t);
    }

    const visibleQuadrants = quadrants.filter((q) => (quadrantIndex.get(q.key) ?? []).length > 0);

    // 用 template-areas 让“剩余象限”自动铺满，避免空洞。
    const visibleKeys = new Set<QuadrantKey>(visibleQuadrants.map((q) => q.key));
    const matrixAreas = computeMatrixTemplateAreas(visibleKeys);

    const matrixHtml = visibleQuadrants
        .map((q) => {
            const list = quadrantIndex.get(q.key) ?? [];
            const items = list.map((t) => renderTaskItem(t, viewMode)).join('');
            return `
        <section class="quadrant" style="grid-area: ${q.key};" data-quadrant="${q.key}">
          <div class="quadrant-header">
            <div class="quadrant-title">${escapeHtml(q.title)}</div>
            <div class="quadrant-meta">
              <span class="pill">${list.length}</span>
              <button class="btn btn-ghost btn-icon" data-action="add-task" data-important="${
                q.important ? '1' : '0'
            }" data-urgent="${q.urgent ? '1' : '0'}" title="在此象限新增任务">+</button>
            </div>
          </div>
           <div class="task-list ${viewMode}">
            ${items}
           </div>
         </section>
       `;
        })
        .join('');

    let modalHtml = '';
    const modal = state.modal;
    if (modal?.kind === 'task') {
        const statusOptions = statusValues
            .map((st) => {
                const selected = String(modal.status) === st ? 'selected' : '';
                return `<option value="${st}" ${selected}>${statusLabels[st] ?? st}</option>`;
            })
            .join('');
        const deleteBtn =
            modal.id > 0
                ? `<button class="btn btn-danger" type="button" data-action="delete-task" data-task-id="${Number(
                    modal.id,
                )}">删除</button>`
                : '';
        modalHtml = `
      <div class="modal-backdrop" data-action="close-modal"></div>
      <div class="modal" role="dialog" aria-modal="true" aria-label="任务">
        <div class="modal-title-row">
          <div class="modal-title">${modal.id ? '编辑任务' : '新增任务'}</div>
          ${deleteBtn}
        </div>
        <form data-action="submit-task">
          <label class="field">
            <div class="field-label">标题</div>
            <input class="input" data-focus="task-title" name="title" value="${escapeHtml(
            modal.title,
        )}" maxlength="200" autocomplete="off" />
          </label>
          <label class="field">
            <div class="field-label">内容（可选）</div>
            <textarea class="textarea" name="content" rows="3" maxlength="1000">${escapeHtml(
            modal.content,
        )}</textarea>
          </label>
          <div class="grid2">
            <label class="toggle toggle-plain">
              <input type="checkbox" class="checkbox" name="important" ${
            modal.important ? 'checked' : ''
        } />
              <span>重要</span>
            </label>
            <label class="toggle toggle-plain">
              <input type="checkbox" class="checkbox" name="urgent" ${
            modal.urgent ? 'checked' : ''
        } />
              <span>紧急</span>
            </label>
          </div>
          <label class="field">
            <div class="field-label">状态</div>
            <select class="select" name="status">${statusOptions}</select>
          </label>
          ${state.modalError ? `<div class="error">${escapeHtml(state.modalError)}</div>` : ''}
          <div class="modal-actions">
            <button class="btn btn-ghost" type="button" data-action="close-modal">取消</button>
            <button class="btn btn-primary" type="submit">保存</button>
          </div>
        </form>
      </div>
    `;
    }
    if (modal?.kind === 'confirm') {
        const confirmBtnClass = modal.danger ? 'btn-danger' : 'btn-primary';
        modalHtml = `
      <div class="modal-backdrop" data-action="close-modal"></div>
      <div class="modal" role="dialog" aria-modal="true" aria-label="确认">
        <div class="modal-title">${escapeHtml(modal.title)}</div>
        <div class="confirm-message">${escapeHtml(modal.message)}</div>
        ${state.modalError ? `<div class="error">${escapeHtml(state.modalError)}</div>` : ''}
        <div class="modal-actions">
          <button class="btn btn-ghost" type="button" data-action="close-modal" ${
            modal.pending ? 'disabled' : ''
        }>取消</button>
          <button class="btn ${confirmBtnClass}" type="button" data-action="confirm-ok" data-focus="confirm-ok" ${
            modal.pending ? 'disabled' : ''
        }>${
            modal.pending ? '处理中…' : escapeHtml(modal.confirmText)
        }</button>
        </div>
      </div>
    `;
    }
    if (modal?.kind === 'update') {
        const updateInfo = modal.updateInfo;
        const release = updateInfo.latestRelease;
        if (!release) return '';

        // 将 Markdown 风格的更新内容转换为简单的 HTML（截取前 500 字符）
        let description = String(release.description ?? '暂无更新说明').trim();
        if (description.length > 500) {
            description = description.substring(0, 500) + '...';
        }
        // 简单处理换行
        description = description.replaceAll('\n', '<br>');

        modalHtml = `
      <div class="modal-backdrop" data-action="close-modal"></div>
      <div class="modal modal-update" role="dialog" aria-modal="true" aria-label="发现新版本">
        <div class="modal-title">🎉 发现新版本</div>
        <div class="update-info">
          <div class="update-version">
            <span class="label">当前版本:</span> <span class="version">${escapeHtml(updateInfo.currentVersion)}</span>
          </div>
          <div class="update-version">
            <span class="label">最新版本:</span> <span class="version version-new">${escapeHtml(release.version)}</span>
          </div>
          <div class="update-name">${escapeHtml(release.name)}</div>
          <div class="update-description">${description}</div>
        </div>
        ${state.modalError ? `<div class="error">${escapeHtml(state.modalError)}</div>` : ''}
        <div class="modal-actions">
          <button class="btn btn-ghost" type="button" data-action="close-modal" ${
            modal.pending ? 'disabled' : ''
        }>稍后提醒</button>
          <button class="btn btn-ghost" type="button" data-action="view-release" ${
            modal.pending ? 'disabled' : ''
        }>查看详情</button>
          <button class="btn btn-primary" type="button" data-action="download-update" data-focus="download-update" ${
            modal.pending ? 'disabled' : ''
        }>立即下载</button>
        </div>
      </div>
    `;
    }

    appEl.innerHTML = `
     <div class="app-shell">
       ${
        state.loading
            ? '<div class="loading page-pad">加载中…</div>'
            : state.error
                ? `<div class="error-block page-pad">加载失败：${escapeHtml(state.error)}</div>`
                : matrixAreas
                    ? `<div class="matrix" style="grid-template-areas: ${matrixAreas};">${matrixHtml}</div>`
                    : '<div class="empty-state page-pad">暂无任务，点击右下角 + 新建</div>'
    }

       ${menuAllowed ? '<button class="fab fab-menu" data-action="toggle-menu" aria-label="菜单">≡</button>' : ''}
       <button class="fab fab-add" data-action="add-task" title="新增任务" aria-label="新增任务">+</button>

      ${state.drawerOpen && menuAllowed ? renderDrawer(settings) : ''}
      ${state.modal ? `<div class="modal-root">${modalHtml}</div>` : ''}
       ${
        state.toast
            ? `<div class="toast toast-${state.toast.kind} toast-${state.toast.position ?? 'corner'}" data-action="dismiss-toast" role="status">${escapeHtml(
                state.toast.message,
            )}</div>`
            : ''
    }
    </div>
	`;
}

// 事件委托：所有按钮通过 `data-action` 声明操作，统一在这里分发。
appEl.addEventListener('click', async (e) => {
    const el = (e.target as HTMLElement | null)?.closest?.('[data-action]') as HTMLElement | null;
    if (!el) return;

    const action = el.getAttribute('data-action');
    if (!action) return;
    try {
        switch (action) {
            case 'toggle-menu':
                if (!isMenuAllowed()) {
                    state.drawerOpen = false;
                    render();
                    break;
                }
                state.drawerOpen = !state.drawerOpen;
                render();
                break;
            case 'set-view-mode': {
                const mode = el.getAttribute('data-view-mode');
                if (!mode) return;
                const settings = await SetViewMode(mode);
                if (state.board) state.board.settings = settings;
                render();
                break;
            }
            case 'add-task': {
                const hasPreset =
                    el.hasAttribute('data-important') || el.hasAttribute('data-urgent');
                const preset = hasPreset
                    ? {
                        important: el.getAttribute('data-important') === '1',
                        urgent: el.getAttribute('data-urgent') === '1',
                    }
                    : state.lastPreset;
                state.lastPreset = preset;
                openTaskModal(null, preset);
                break;
            }
            case 'edit-task': {
                const taskId = Number(el.getAttribute('data-task-id'));
                const task = getTaskById(taskId);
                if (!task) return;
                openTaskModal(task);
                break;
            }
            case 'delete-task': {
                const taskId = Number(el.getAttribute('data-task-id'));
                if (!Number.isFinite(taskId) || taskId <= 0) return;
                const task = getTaskById(taskId);
                openConfirmModal({
                    title: '删除任务',
                    message: task?.title ? `确定删除任务「${task.title}」？` : '确定删除该任务？',
                    targetType: 'task',
                    targetId: taskId,
                    confirmText: '删除',
                    danger: true,
                });
                break;
            }
            case 'confirm-ok': {
                const modal = state.modal;
                if (modal?.kind !== 'confirm') return;
                state.modalError = null;
                state.modal = {...modal, pending: true};
                render();

                try {
                    const targetType = modal.targetType;
                    const targetId = Number(modal.targetId);
                    if (targetType === 'task') {
                        await DeleteTask(targetId);
                    }
                    await refresh();
                    closeModal();
                } catch (err) {
                    console.error(err);
                    state.modal = {...modal, pending: false};
                    state.modalError = formatError(err);
                    render();
                }
                break;
            }
            case 'close-modal':
                closeModal();
                break;
            case 'dismiss-toast':
                state.toast = null;
                render();
                break;
            case 'quit-app':
                await Quit();
                break;
            case 'check-update':
                // await checkForUpdates(true);
                showToast('暂未开发完成','error')
                break;
            case 'download-update': {
                const modal = state.modal;
                if (modal?.kind !== 'update') return;
                const downloadURL = modal.updateInfo.latestRelease?.downloadUrl;
                if (!downloadURL) {
                    showToast('未找到下载链接', 'error');
                    return;
                }
                try {
                    await OpenURL(downloadURL);
                    showToast('已在浏览器中打开下载页面', 'success');
                    closeModal();
                } catch (err) {
                    console.error(err);
                    showToast(formatError(err), 'error');
                }
                break;
            }
            case 'view-release': {
                const modal = state.modal;
                if (modal?.kind !== 'update') return;
                const pageUrl = modal.updateInfo.latestRelease?.pageUrl;
                if (!pageUrl) {
                    showToast('未找到详情页面链接', 'error',2500,'center');
                    return;
                }
                try {
                    await OpenURL(pageUrl);
                    closeModal();
                } catch (err) {
                    console.error(err);
                    showToast(formatError(err), 'error');
                }
                break;
            }
        }
    } catch (err) {
        console.error(err);
        showToast(formatError(err));
    }
});

// 弹窗输入时同步写回 state，避免 re-render 覆盖用户正在输入的内容。
appEl.addEventListener('input', (e) => {
    if (!state.modal) return;

    const el = e.target;
    if (!(el instanceof HTMLInputElement || el instanceof HTMLTextAreaElement)) return;
    if (!el.closest('.modal')) return;

    if (state.modal.kind === 'task') {
        if (el.name === 'title') {
            state.modal.title = el.value;
            return;
        }
        if (el.name === 'content') {
            state.modal.content = el.value;
        }
    }
});

// change 事件：处理 checkbox/select 的变化（设置项、任务完成勾选等）。
appEl.addEventListener('change', async (e) => {
    const el = e.target;
    if (!(el instanceof HTMLElement)) return;

    if (state.modal?.kind === 'task' && el.closest('.modal')) {
        if (el instanceof HTMLSelectElement && el.name === 'status') {
            state.modal.status = normalizeStatusValue(el.value);
            return;
        }
        if (el instanceof HTMLInputElement && el.name === 'important') {
            state.modal.important = !!el.checked;
            return;
        }
        if (el instanceof HTMLInputElement && el.name === 'urgent') {
            state.modal.urgent = !!el.checked;
            return;
        }
    }

    const action = el.getAttribute('data-action');
    if (!action) return;

    try {
        switch (action) {
            case 'toggle-theme': {
                if (!(el instanceof HTMLInputElement)) return;
                const rect = el.getBoundingClientRect();
                const origin = {x: rect.left + rect.width / 2, y: rect.top + rect.height / 2};
                const nextTheme: Theme = el.checked ? 'dark' : 'light';
                const prevTheme = getCurrentTheme();

                el.disabled = true;
                try {
                    const settings = await SetTheme(nextTheme);
                    if (state.board) state.board.settings = settings;
                    persistTheme(nextTheme);
                    await animateThemeTransition(nextTheme, origin);
                } catch (err) {
                    el.checked = prevTheme === 'dark';
                    persistTheme(prevTheme);
                    setDocumentTheme(prevTheme);
                    throw err;
                } finally {
                    el.disabled = false;
                }
                break;
            }
            case 'toggle-hide-done': {
                if (!(el instanceof HTMLInputElement)) return;
                const settings = await SetHideDone(!!el.checked);
                if (state.board) state.board.settings = settings;

                render();
                break;
            }
            case 'toggle-always-on-top': {
                if (!(el instanceof HTMLInputElement)) return;
                const settings = await SetAlwaysOnTop(!!el.checked);
                if (state.board) state.board.settings = settings;
                render();
                break;
            }
            case 'toggle-concise-mode': {
                if (!(el instanceof HTMLInputElement)) return;
                const settings = await SetConciseMode(!!el.checked);
                if (state.board) state.board.settings = settings;
                render();
                // 简洁模式需要重启应用才能生效，自动重启
                showToast('简洁模式已保存，正在重启应用...', 'success', 2000);
                // 延迟重启，让用户看到提示
                setTimeout(async () => {
                    try {
                        await Restart();
                    } catch (err) {
                        console.error('重启失败:', err);
                        showToast('自动重启失败，请手动重启应用', 'error', 5000);
                    }
                }, 1000);
                break;
            }
            case 'toggle-task-done': {
                if (!(el instanceof HTMLInputElement)) return;
                const taskId = Number(el.getAttribute('data-task-id'));
                const task = getTaskById(taskId);
                if (!task) return;
                const nextStatus = el.checked ? 'done' : 'todo';
                await UpsertTask({...task, status: nextStatus});
                await refresh();
                break;
            }
        }
    } catch (err) {
        console.error(err);
        showToast(formatError(err));
    }
});

// submit 事件：提交“新增/编辑任务”表单。
appEl.addEventListener('submit', async (e) => {
    const form = e.target;
    if (!(form instanceof HTMLFormElement)) return;
    const action = form.getAttribute('data-action');
    if (!action) return;

    e.preventDefault();
    state.modalError = null;

    try {
        if (action === 'submit-task') {
            const modal = state.modal;
            if (!modal || modal.kind !== 'task') return;

            const groupId = Number(modal.groupId ?? getDefaultGroupId());
            if (!Number.isFinite(groupId) || groupId <= 0) {
                throw new Error('初始化未完成，请稍后重试');
            }

            const statusRaw = String(
                (form.elements.namedItem('status') as HTMLSelectElement | null)?.value ?? modal.status ?? 'todo',
            );
            if (!isStatusValue(statusRaw)) throw new Error(`无效的任务状态: ${statusRaw}`);
            const status: StatusValue = statusRaw;

            const title = String(
                (form.elements.namedItem('title') as HTMLInputElement | null)?.value ?? '',
            ).trim();
            if (!title) {
                throw new Error('任务标题不能为空');
            }
            if (Array.from(title).length > 200) {
                throw new Error('任务标题过长（最多 200 字）');
            }

            const content = String(
                (form.elements.namedItem('content') as HTMLTextAreaElement | null)?.value ?? '',
            ).trim();
            if (Array.from(content).length > 1000) {
                throw new Error('任务内容过长（最多 1000 字）');
            }

            const important = !!(form.elements.namedItem('important') as HTMLInputElement | null)?.checked;
            const urgent = !!(form.elements.namedItem('urgent') as HTMLInputElement | null)?.checked;

            await UpsertTask({
                id: Number(modal.id ?? 0),
                groupId,
                status,
                title,
                content,
                important,
                urgent,
                createdAt: 0,
                updatedAt: 0,
            });

            closeModal();
            await refresh();
            showToast('已保存', 'success');
        }
    } catch (err) {
        state.modalError = formatError(err);
        render();
    }
});

// Esc：优先关闭弹窗，其次关闭菜单。
document.addEventListener('keydown', (e) => {
    if (e.key === 'Escape') {
        if (state.modal) {
            e.preventDefault();
            closeModal();
            return;
        }
        if (state.drawerOpen) {
            e.preventDefault();
            state.drawerOpen = false;
            render();
        }
    }
});

// 窗口变小到阈值后，自动关闭菜单并隐藏入口。
window.addEventListener('resize', () => {
    if (resizeRaf) cancelAnimationFrame(resizeRaf);
    resizeRaf = requestAnimationFrame(() => {
        resizeRaf = null;
        if (!isMenuAllowed()) state.drawerOpen = false;
        render();
    });
});

// checkForUpdates 检查应用更新
async function checkForUpdates(showNoUpdateMessage = false) {
    try {
        const result = await CheckUpdate();
        if (result.hasUpdate && result.latestRelease) {
            // 有更新，显示更新弹窗
            state.modalError = null;
            state.modal = {
                kind: 'update',
                updateInfo: result,
                pending: false,
            };
            render();
        } else if (showNoUpdateMessage) {
            // 手动检查时，如果没有更新则提示
            showToast('当前已是最新版本', 'success');
        }
    } catch (err) {
        console.error('检查更新失败:', err);
        if (showNoUpdateMessage) {
            // 手动检查时显示错误
            showToast('检查更新失败，请稍后重试', 'error');
        }
        // 启动时自动检查失败则静默忽略
    }
}

// 首次渲染：先出骨架，再刷新数据。
setDocumentTheme(loadStoredTheme() ?? 'light');
render();
refresh();
startWaterReminder();

// 启动时检查更新（延迟 3 秒，避免影响应用启动速度）
setTimeout(() => {
    checkForUpdates(false);
}, 3000);

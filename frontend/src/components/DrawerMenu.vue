<template>
    <div class="drawer-root" :data-state="phase">
        <div class="drawer-backdrop" @click="emit('close')"></div>
        <aside class="drawer" role="dialog" aria-label="菜单" aria-modal="true" @animationend="onAnimationEnd">
            <div class="drawer-title">菜单</div>

            <div class="drawer-section">
                <div class="drawer-section-title">视图</div>
                <div class="seg">
                    <button
                        class="btn"
                        :class="{ 'btn-primary': viewMode === 'cards' }"
                        type="button"
                        @click="emit('setViewMode', 'cards')"
                    >
                        卡片
                    </button>
                    <button
                        class="btn"
                        :class="{ 'btn-primary': viewMode === 'list' }"
                        type="button"
                        @click="emit('setViewMode', 'list')"
                    >
                        列表
                    </button>
                </div>
            </div>

            <div class="drawer-section">
                <label class="toggle">
                    <input
                        type="checkbox"
                        class="checkbox"
                        :checked="!!settings.hideDone"
                        @change="onToggle($event, 'hideDone')"
                    />
                    <span>隐藏已完成</span>
                </label>
                <label class="toggle">
                    <input
                        type="checkbox"
                        class="checkbox"
                        :checked="!!settings.alwaysOnTop"
                        @change="onToggle($event, 'alwaysOnTop')"
                    />
                    <span>置顶悬浮</span>
                </label>
                <label class="toggle">
                    <input
                        type="checkbox"
                        class="checkbox"
                        :checked="theme === 'dark'"
                        @change="onToggleTheme"
                    />
                    <span>夜间模式</span>
                </label>
                <label class="toggle">
                    <input
                        type="checkbox"
                        class="checkbox"
                        :checked="!!settings.conciseMode"
                        @change="onToggle($event, 'conciseMode')"
                    />
                    <span>简洁模式</span>
                </label>
            </div>

            <div class="drawer-section">
                <button class="btn btn-ghost" type="button" @click="emit('checkUpdates')">
                    检查更新
                </button>
                <button class="btn btn-ghost" type="button" @click="emit('quit')">退出应用</button>
                <button class="btn btn-ghost" type="button" @click="emit('close')">关闭菜单</button>
            </div>
        </aside>
    </div>
</template>

<script setup lang="ts">
import { toRefs } from 'vue';

import type { todo } from '../../wailsjs/go/models';

import type { Theme } from '../theme';
import type { ViewMode } from '../types';

type Phase = 'open' | 'closing';

const props = defineProps<{
    phase: Phase;
    settings: todo.Settings;
    viewMode: ViewMode;
    theme: Theme;
}>();

const { phase, settings, theme, viewMode } = toRefs(props);

const emit = defineEmits<{
    (e: 'close'): void;
    (e: 'closed'): void;
    (e: 'setViewMode', mode: ViewMode): void;
    (e: 'toggleHideDone', checked: boolean): void;
    (e: 'toggleAlwaysOnTop', checked: boolean): void;
    (e: 'toggleTheme', payload: { checked: boolean; origin: { x: number; y: number } }): void;
    (e: 'toggleConciseMode', checked: boolean): void;
    (e: 'checkUpdates'): void;
    (e: 'quit'): void;
}>();

function onAnimationEnd(e: AnimationEvent) {
    if (phase.value !== 'closing') return;
    if (e.animationName !== 'drawer-out') return;
    emit('closed');
}

function onToggleTheme(e: Event) {
    const el = e.target;
    if (!(el instanceof HTMLInputElement)) return;

    const rect = el.getBoundingClientRect();
    const origin = { x: rect.left + rect.width / 2, y: rect.top + rect.height / 2 };
    emit('toggleTheme', { checked: el.checked, origin });
}

function onToggle(e: Event, type: 'hideDone' | 'alwaysOnTop' | 'conciseMode') {
    const el = e.target;
    if (!(el instanceof HTMLInputElement)) return;

    if (type === 'hideDone') emit('toggleHideDone', el.checked);
    if (type === 'alwaysOnTop') emit('toggleAlwaysOnTop', el.checked);
    if (type === 'conciseMode') emit('toggleConciseMode', el.checked);
}
</script>

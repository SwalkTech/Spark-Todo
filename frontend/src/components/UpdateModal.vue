<template>
    <div class="modal modal-update" role="dialog" aria-modal="true" aria-label="发现新版本" @click.stop>
        <div class="modal-title">发现新版本</div>
        <div class="update-info">
            <div class="update-version">
                <span class="label">当前版本:</span>
                <span class="version">{{ updateInfo.currentVersion }}</span>
            </div>
            <div class="update-version">
                <span class="label">最新版本:</span>
                <span class="version version-new">{{ updateInfo.latestRelease?.version }}</span>
            </div>
            <div class="update-name">{{ updateInfo.latestRelease?.name }}</div>
            <div class="update-description">{{ description }}</div>
        </div>

        <div v-if="error" class="error">{{ error }}</div>

        <div class="modal-actions">
            <button class="btn btn-ghost" type="button" :disabled="pending" @click="emit('close')">
                稍后提醒
            </button>
            <button class="btn btn-ghost" type="button" :disabled="pending" @click="emit('viewRelease')">
                查看详情
            </button>
            <button class="btn btn-primary" type="button" :disabled="pending" @click="emit('download')">
                立即下载
            </button>
        </div>
    </div>
</template>

<script setup lang="ts">
import type { version } from '../../wailsjs/go/models';

defineProps<{
    updateInfo: version.UpdateCheckResult;
    pending: boolean;
    description: string;
    error: string | null;
}>();

const emit = defineEmits<{
    (e: 'close'): void;
    (e: 'viewRelease'): void;
    (e: 'download'): void;
}>();
</script>


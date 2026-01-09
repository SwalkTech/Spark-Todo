import type { version } from '../wailsjs/go/models';

export type StatusValue = 'todo' | 'doing' | 'done';
export type ViewMode = 'list' | 'cards';
export type ToastKind = 'error' | 'success';
export type ToastPosition = 'corner' | 'center';
export type QuadrantKey = 'iu' | 'in' | 'nu' | 'nn';

export type QuadrantPreset = { important: boolean; urgent: boolean };

export type TaskModalState = {
    kind: 'task';
    id: number;
    groupId: number;
    parentId: number;
    title: string;
    content: string;
    status: StatusValue;
    important: boolean;
    urgent: boolean;
};

export type ConfirmModalState = {
    kind: 'confirm';
    title: string;
    message: string;
    targetType: 'task';
    targetId: number;
    confirmText: string;
    danger: boolean;
    pending: boolean;
};

export type UpdateModalState = {
    kind: 'update';
    updateInfo: version.UpdateCheckResult;
    pending: boolean;
};

export type ModalState = TaskModalState | ConfirmModalState | UpdateModalState | null;

export type ToastState = {
    kind: ToastKind;
    message: string;
    position: ToastPosition;
};


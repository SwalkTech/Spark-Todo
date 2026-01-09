export type Theme = 'light' | 'dark';

export const THEME_STORAGE_KEY = 'sparkTodoTheme';

export function normalizeTheme(value: unknown): Theme {
    const v = String(value ?? '').trim().toLowerCase();
    return v === 'dark' ? 'dark' : 'light';
}

export function getCurrentTheme(): Theme {
    return normalizeTheme(document.documentElement.getAttribute('data-theme'));
}

export function setDocumentTheme(theme: Theme) {
    document.documentElement.setAttribute('data-theme', theme);
    document.documentElement.style.colorScheme = theme === 'dark' ? 'dark' : 'light';
}

export function loadStoredTheme(): Theme | null {
    try {
        const raw = localStorage.getItem(THEME_STORAGE_KEY);
        if (!raw) return null;
        return normalizeTheme(raw);
    } catch {
        return null;
    }
}

export function persistTheme(theme: Theme) {
    try {
        localStorage.setItem(THEME_STORAGE_KEY, theme);
    } catch {
        // ignore
    }
}

function resolveCssVar(value: string, styles: CSSStyleDeclaration): string {
    let current = String(value ?? '').trim();
    for (let i = 0; i < 6; i++) {
        const match = current.match(/^var\(\s*(--[^,\s)]+)\s*(?:,\s*([^)]+))?\)$/);
        if (!match) return current;
        const next = styles.getPropertyValue(match[1]).trim();
        if (next) {
            current = next;
            continue;
        }
        const fallback = match[2]?.trim();
        if (fallback) {
            current = fallback;
            continue;
        }
        return '';
    }
    return current;
}

function parseDurationMs(value: string, fallbackMs: number): number {
    const raw = String(value ?? '').trim();
    if (!raw) return fallbackMs;

    const n = Number.parseFloat(raw);
    if (!Number.isFinite(n)) return fallbackMs;

    if (raw.endsWith('ms')) return Math.max(0, n);
    if (raw.endsWith('s')) return Math.max(0, n * 1000);
    return Math.max(0, n);
}

export async function animateThemeTransition(nextTheme: Theme, origin: {x: number; y: number}) {
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

    const styles = getComputedStyle(document.documentElement);
    const durationMs = parseDurationMs(
        resolveCssVar(styles.getPropertyValue('--theme-transition-duration'), styles),
        450,
    );
    const easing =
        resolveCssVar(styles.getPropertyValue('--theme-transition-easing'), styles) || 'cubic-bezier(0.2, 0, 0, 1)';

    const anim = (document.documentElement as any).animate(
        {
            clipPath: [
                `circle(0px at ${origin.x}px ${origin.y}px)`,
                `circle(${endRadius}px at ${origin.x}px ${origin.y}px)`,
            ],
        },
        {
            duration: durationMs,
            easing,
            pseudoElement: '::view-transition-new(root)',
        },
    );
    await anim.finished.catch(() => {
        // ignore
    });
}

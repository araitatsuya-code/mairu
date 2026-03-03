export type RuntimeStatus = {
    authorized: boolean;
    claudeConfigured: boolean;
    databaseReady: boolean;
    lastRunAt: string | null;
};

type WailsAppApi = {
    AppName?: () => Promise<string> | string;
    GetRuntimeStatus?: () =>
        | Promise<{
              Authorized: boolean;
              ClaudeConfigured: boolean;
              DatabaseReady: boolean;
              LastRunAt?: string | null;
          }>
        | {
              Authorized: boolean;
              ClaudeConfigured: boolean;
              DatabaseReady: boolean;
              LastRunAt?: string | null;
          };
};

declare global {
    interface Window {
        go?: {
            main?: {
                App?: WailsAppApi;
            };
        };
    }
}

export const defaultRuntimeStatus: RuntimeStatus = {
    authorized: false,
    claudeConfigured: false,
    databaseReady: false,
    lastRunAt: null,
};

export async function loadAppName(): Promise<string> {
    const appApi = window.go?.main?.App;
    const result = appApi?.AppName?.();
    if (typeof result === 'string') {
        return result;
    }
    if (result) {
        return result;
    }
    return 'Mairu';
}

export async function loadRuntimeStatus(): Promise<RuntimeStatus> {
    const appApi = window.go?.main?.App;
    const result = appApi?.GetRuntimeStatus?.();
    const raw = result ? await result : null;

    if (!raw) {
        return defaultRuntimeStatus;
    }

    return {
        authorized: raw.Authorized,
        claudeConfigured: raw.ClaudeConfigured,
        databaseReady: raw.DatabaseReady,
        lastRunAt: raw.LastRunAt ?? null,
    };
}

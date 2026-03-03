export type RuntimeStatus = {
    authorized: boolean;
    googleConfigured: boolean;
    authStatus: string;
    claudeConfigured: boolean;
    databaseReady: boolean;
    lastRunAt: string | null;
};

export type GoogleLoginResult = {
    success: boolean;
    message: string;
    authorizationURL: string;
    redirectURL: string;
    codePreview: string;
    scopes: string[];
};

type WailsAppApi = {
    AppName?: () => Promise<string> | string;
    GetRuntimeStatus?: () =>
        | Promise<{
              Authorized: boolean;
              GoogleConfigured: boolean;
              AuthStatus: string;
              ClaudeConfigured: boolean;
              DatabaseReady: boolean;
              LastRunAt?: string | null;
          }>
        | {
              Authorized: boolean;
              GoogleConfigured: boolean;
              AuthStatus: string;
              ClaudeConfigured: boolean;
              DatabaseReady: boolean;
              LastRunAt?: string | null;
          };
    StartGoogleLogin?: () =>
        | Promise<{
              Success: boolean;
              Message: string;
              AuthorizationURL: string;
              RedirectURL: string;
              CodePreview: string;
              Scopes?: string[];
          }>
        | {
              Success: boolean;
              Message: string;
              AuthorizationURL: string;
              RedirectURL: string;
              CodePreview: string;
              Scopes?: string[];
          };
    CancelGoogleLogin?: () => Promise<boolean> | boolean;
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
    googleConfigured: false,
    authStatus: 'Google ログイン設定を確認しています。',
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
        googleConfigured: raw.GoogleConfigured,
        authStatus: raw.AuthStatus,
        claudeConfigured: raw.ClaudeConfigured,
        databaseReady: raw.DatabaseReady,
        lastRunAt: raw.LastRunAt ?? null,
    };
}

export async function startGoogleLogin(): Promise<GoogleLoginResult> {
    const appApi = window.go?.main?.App;
    const result = appApi?.StartGoogleLogin?.();

    if (!result) {
        throw new Error('Google ログイン API がまだ公開されていません。');
    }

    const raw = await result;
    return {
        success: raw.Success,
        message: raw.Message,
        authorizationURL: raw.AuthorizationURL,
        redirectURL: raw.RedirectURL,
        codePreview: raw.CodePreview,
        scopes: raw.Scopes ?? [],
    };
}

export async function cancelGoogleLogin(): Promise<boolean> {
    const appApi = window.go?.main?.App;
    const result = appApi?.CancelGoogleLogin?.();

    if (typeof result === 'boolean') {
        return result;
    }
    if (!result) {
        return false;
    }
    return await result;
}

export type RuntimeStatus = {
    authorized: boolean;
    googleConfigured: boolean;
    authStatus: string;
    claudeConfigured: boolean;
    claudeStatus: string;
    databaseReady: boolean;
    lastRunAt: string | null;
};

export type GoogleLoginResult = {
    success: boolean;
    message: string;
    authorizationURL: string;
    redirectURL: string;
    tokenStored: boolean;
    refreshTokenStored: boolean;
    storedPreview: string;
    scopes: string[];
};

export type SecretOperationResult = {
    success: boolean;
    message: string;
};

type WailsAppApi = {
    AppName?: () => Promise<string> | string;
    GetRuntimeStatus?: () =>
        | Promise<{
              Authorized: boolean;
              GoogleConfigured: boolean;
              AuthStatus: string;
              ClaudeConfigured: boolean;
              ClaudeStatus: string;
              DatabaseReady: boolean;
              LastRunAt?: string | null;
          }>
        | {
              Authorized: boolean;
              GoogleConfigured: boolean;
              AuthStatus: string;
              ClaudeConfigured: boolean;
              ClaudeStatus: string;
              DatabaseReady: boolean;
              LastRunAt?: string | null;
          };
    StartGoogleLogin?: () =>
        | Promise<{
              Success: boolean;
              Message: string;
              AuthorizationURL: string;
              RedirectURL: string;
              TokenStored: boolean;
              RefreshTokenStored: boolean;
              StoredPreview: string;
              Scopes?: string[];
          }>
        | {
              Success: boolean;
              Message: string;
              AuthorizationURL: string;
              RedirectURL: string;
              TokenStored: boolean;
              RefreshTokenStored: boolean;
              StoredPreview: string;
              Scopes?: string[];
          };
    CancelGoogleLogin?: () => Promise<boolean> | boolean;
    SaveClaudeAPIKey?: (apiKey: string) =>
        | Promise<{
              Success: boolean;
              Message: string;
          }>
        | {
              Success: boolean;
              Message: string;
          };
    ClearClaudeAPIKey?: () =>
        | Promise<{
              Success: boolean;
              Message: string;
          }>
        | {
              Success: boolean;
              Message: string;
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
    googleConfigured: false,
    authStatus: 'Google ログイン設定を確認しています。',
    claudeConfigured: false,
    claudeStatus: 'Claude API キー状態を確認しています。',
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
        claudeStatus: raw.ClaudeStatus,
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
        tokenStored: raw.TokenStored,
        refreshTokenStored: raw.RefreshTokenStored,
        storedPreview: raw.StoredPreview,
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

export async function saveClaudeAPIKey(apiKey: string): Promise<SecretOperationResult> {
    const appApi = window.go?.main?.App;
    const result = appApi?.SaveClaudeAPIKey?.(apiKey);

    if (!result) {
        throw new Error('Claude API キー保存 API がまだ公開されていません。');
    }

    const raw = await result;
    return {
        success: raw.Success,
        message: raw.Message,
    };
}

export async function clearClaudeAPIKey(): Promise<SecretOperationResult> {
    const appApi = window.go?.main?.App;
    const result = appApi?.ClearClaudeAPIKey?.();

    if (!result) {
        throw new Error('Claude API キー削除 API がまだ公開されていません。');
    }

    const raw = await result;
    return {
        success: raw.Success,
        message: raw.Message,
    };
}

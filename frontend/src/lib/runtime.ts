export type RuntimeStatus = {
    authorized: boolean;
    googleConfigured: boolean;
    authStatus: string;
    googleTokenPreview: string;
    gmailConnected: boolean;
    gmailStatus: string;
    gmailAccountEmail: string;
    claudeConfigured: boolean;
    claudeStatus: string;
    claudeKeyPreview: string;
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

export type EmailSummary = {
    id: string;
    threadID: string;
    from: string;
    subject: string;
    snippet: string;
    unread: boolean;
};

export type ClassificationCategory =
    | 'important'
    | 'newsletter'
    | 'junk'
    | 'archive'
    | 'unread_priority';

export type ClassificationReviewLevel =
    | 'auto_apply'
    | 'review'
    | 'review_with_reason'
    | 'hold';

export type ClassificationRequest = {
    model?: string;
    messages: EmailSummary[];
};

export type ClassificationResult = {
    messageID: string;
    category: ClassificationCategory;
    confidence: number;
    reason: string;
    reviewLevel: ClassificationReviewLevel;
};

export type ClassificationResponse = {
    model: string;
    results: ClassificationResult[];
};

export type GmailConnectionResult = {
    success: boolean;
    message: string;
    emailAddress: string;
    messagesTotal: number;
    threadsTotal: number;
    historyID: string;
    tokenRefreshed: boolean;
};

type WailsAppApi = {
    AppName?: () => Promise<string> | string;
    GetRuntimeStatus?: () =>
        | Promise<{
              Authorized: boolean;
              GoogleConfigured: boolean;
              AuthStatus: string;
              GoogleTokenPreview?: string;
              GmailConnected?: boolean;
              GmailStatus?: string;
              GmailAccountEmail?: string;
              ClaudeConfigured: boolean;
              ClaudeStatus: string;
              ClaudeKeyPreview?: string;
              DatabaseReady: boolean;
              LastRunAt?: string | null;
          }>
        | {
              Authorized: boolean;
              GoogleConfigured: boolean;
              AuthStatus: string;
              GoogleTokenPreview?: string;
              GmailConnected?: boolean;
              GmailStatus?: string;
              GmailAccountEmail?: string;
              ClaudeConfigured: boolean;
              ClaudeStatus: string;
              ClaudeKeyPreview?: string;
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
    ClassifyEmails?: (request: {
        Model?: string;
        Messages: Array<{
            ID: string;
            ThreadID?: string;
            From: string;
            Subject: string;
            Snippet: string;
            Unread: boolean;
        }>;
    }) =>
        | Promise<{
              Model?: string;
              Results?: Array<{
                  MessageID: string;
                  Category: ClassificationCategory;
                  Confidence: number;
                  Reason: string;
                  ReviewLevel: ClassificationReviewLevel;
              }>;
          }>
        | {
              Model?: string;
              Results?: Array<{
                  MessageID: string;
                  Category: ClassificationCategory;
                  Confidence: number;
                  Reason: string;
                  ReviewLevel: ClassificationReviewLevel;
              }>;
          };
    CheckGmailConnection?: () =>
        | Promise<{
              Success: boolean;
              Message: string;
              EmailAddress?: string;
              MessagesTotal?: number;
              ThreadsTotal?: number;
              HistoryID?: string;
              TokenRefreshed?: boolean;
          }>
        | {
              Success: boolean;
              Message: string;
              EmailAddress?: string;
              MessagesTotal?: number;
              ThreadsTotal?: number;
              HistoryID?: string;
              TokenRefreshed?: boolean;
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
    googleTokenPreview: '',
    gmailConnected: false,
    gmailStatus: 'Google ログイン後に Gmail 接続確認を実行できます。',
    gmailAccountEmail: '',
    claudeConfigured: false,
    claudeStatus: 'Claude API キー状態を確認しています。',
    claudeKeyPreview: '',
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
        googleTokenPreview: raw.GoogleTokenPreview ?? '',
        gmailConnected: raw.GmailConnected ?? false,
        gmailStatus: raw.GmailStatus ?? 'Google ログイン後に Gmail 接続確認を実行できます。',
        gmailAccountEmail: raw.GmailAccountEmail ?? '',
        claudeConfigured: raw.ClaudeConfigured,
        claudeStatus: raw.ClaudeStatus,
        claudeKeyPreview: raw.ClaudeKeyPreview ?? '',
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

export async function classifyEmails(request: ClassificationRequest): Promise<ClassificationResponse> {
    const appApi = window.go?.main?.App;
    const result = appApi?.ClassifyEmails?.({
        Model: request.model,
        Messages: request.messages.map((message) => ({
            ID: message.id,
            ThreadID: message.threadID,
            From: message.from,
            Subject: message.subject,
            Snippet: message.snippet,
            Unread: message.unread,
        })),
    });

    if (!result) {
        throw new Error('メール分類 API がまだ公開されていません。');
    }

    const raw = await result;
    return {
        model: raw.Model ?? request.model ?? '',
        results: (raw.Results ?? []).map((item) => ({
            messageID: item.MessageID,
            category: item.Category,
            confidence: item.Confidence,
            reason: item.Reason,
            reviewLevel: item.ReviewLevel,
        })),
    };
}

export async function checkGmailConnection(): Promise<GmailConnectionResult> {
    const appApi = window.go?.main?.App;
    const result = appApi?.CheckGmailConnection?.();

    if (!result) {
        throw new Error('Gmail 接続確認 API がまだ公開されていません。');
    }

    const raw = await result;
    return {
        success: raw.Success,
        message: raw.Message,
        emailAddress: raw.EmailAddress ?? '',
        messagesTotal: raw.MessagesTotal ?? 0,
        threadsTotal: raw.ThreadsTotal ?? 0,
        historyID: raw.HistoryID ?? '',
        tokenRefreshed: raw.TokenRefreshed ?? false,
    };
}

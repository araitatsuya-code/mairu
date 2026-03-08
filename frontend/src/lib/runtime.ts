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
    source: 'claude' | 'blocklist';
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

export type GmailActionDecision = {
    messageID: string;
    category: ClassificationCategory;
    reviewLevel: ClassificationReviewLevel;
};

export type GmailActionMetadata = {
    messageID: string;
    threadID: string;
    from: string;
    subject: string;
    category: ClassificationCategory;
    confidence: number;
    reviewLevel: ClassificationReviewLevel;
    source: 'claude' | 'blocklist';
};

export type ExecuteGmailActionsRequest = {
    confirmed: boolean;
    decisions: GmailActionDecision[];
    metadata: GmailActionMetadata[];
};

export type GmailActionFailure = {
    messageID: string;
    action: 'label' | 'archive' | 'delete' | 'mark_read';
    error: string;
};

export type ExecuteGmailActionsResult = {
    success: boolean;
    message: string;
    processedCount: number;
    successCount: number;
    failureCount: number;
    deletedCount: number;
    archivedCount: number;
    markedReadCount: number;
    labeledCount: number;
    createdLabels: string[];
    failures: GmailActionFailure[];
    tokenRefreshed: boolean;
};

export type BlocklistKind = 'sender' | 'domain';

export type BlocklistEntry = {
    id: number;
    kind: BlocklistKind;
    pattern: string;
    note: string;
    createdAt: string;
    updatedAt: string;
};

export type BlocklistSuggestion = {
    kind: BlocklistKind;
    pattern: string;
    count: number;
    lastSeenAt: string;
    description: string;
};

export type UpsertBlocklistEntryRequest = {
    kind: BlocklistKind;
    pattern: string;
    note: string;
};

export type BlocklistOperationResult = {
    success: boolean;
    message: string;
};

export type OperationResult = {
    success: boolean;
    message: string;
};

export type SchedulerSettings = {
    classificationIntervalMinutes: number;
    blocklistIntervalMinutes: number;
    knownBlockIntervalMinutes: number;
    notificationsEnabled: boolean;
};

export type UpdateSchedulerSettingsRequest = SchedulerSettings;

export type SchedulerNotification = {
    title: string;
    body: string;
    level: 'info' | 'warning' | 'error';
    jobID: string;
    at: string;
};

export type ClassificationCorrection = {
    messageID: string;
    sender: string;
    originalCategory: ClassificationCategory;
    correctedCategory: ClassificationCategory;
};

export type RecordClassificationRunRequest = {
    messages: EmailSummary[];
    results: ClassificationResult[];
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
    GetSchedulerSettings?: () =>
        | Promise<{
              ClassificationIntervalMinutes?: number;
              BlocklistIntervalMinutes?: number;
              KnownBlockIntervalMinutes?: number;
              NotificationsEnabled?: boolean;
          }>
        | {
              ClassificationIntervalMinutes?: number;
              BlocklistIntervalMinutes?: number;
              KnownBlockIntervalMinutes?: number;
              NotificationsEnabled?: boolean;
          };
    UpdateSchedulerSettings?: (request: {
        ClassificationIntervalMinutes: number;
        BlocklistIntervalMinutes: number;
        KnownBlockIntervalMinutes: number;
        NotificationsEnabled: boolean;
    }) =>
        | Promise<{
              Success: boolean;
              Message: string;
          }>
        | {
              Success: boolean;
              Message: string;
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
                  Source?: 'claude' | 'blocklist';
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
                  Source?: 'claude' | 'blocklist';
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
    ExecuteGmailActions?: (request: {
        Confirmed: boolean;
        Decisions: Array<{
            MessageID: string;
            Category: ClassificationCategory;
            ReviewLevel: ClassificationReviewLevel;
        }>;
        Metadata?: Array<{
            MessageID: string;
            ThreadID?: string;
            From?: string;
            Subject?: string;
            Category: ClassificationCategory;
            Confidence: number;
            ReviewLevel: ClassificationReviewLevel;
            Source?: 'claude' | 'blocklist';
        }>;
    }) =>
        | Promise<{
              Success: boolean;
              Message: string;
              ProcessedCount?: number;
              SuccessCount?: number;
              FailureCount?: number;
              DeletedCount?: number;
              ArchivedCount?: number;
              MarkedReadCount?: number;
              LabeledCount?: number;
              CreatedLabels?: string[];
              Failures?: Array<{
                  MessageID: string;
                  Action: 'label' | 'archive' | 'delete' | 'mark_read';
                  Error: string;
              }>;
              TokenRefreshed?: boolean;
          }>
        | {
              Success: boolean;
              Message: string;
              ProcessedCount?: number;
              SuccessCount?: number;
              FailureCount?: number;
              DeletedCount?: number;
              ArchivedCount?: number;
              MarkedReadCount?: number;
              LabeledCount?: number;
              CreatedLabels?: string[];
              Failures?: Array<{
                  MessageID: string;
                  Action: 'label' | 'archive' | 'delete' | 'mark_read';
                  Error: string;
              }>;
              TokenRefreshed?: boolean;
          };
    GetBlocklistEntries?: () =>
        | Promise<
              Array<{
                  ID: number;
                  Kind: BlocklistKind;
                  Pattern: string;
                  Note: string;
                  CreatedAt: string;
                  UpdatedAt: string;
              }>
          >
        | Array<{
              ID: number;
              Kind: BlocklistKind;
              Pattern: string;
              Note: string;
              CreatedAt: string;
              UpdatedAt: string;
          }>;
    UpsertBlocklistEntry?: (request: {
        Kind: BlocklistKind;
        Pattern: string;
        Note: string;
    }) =>
        | Promise<{
              Success: boolean;
              Message: string;
          }>
        | {
              Success: boolean;
              Message: string;
          };
    DeleteBlocklistEntry?: (id: number) =>
        | Promise<{
              Success: boolean;
              Message: string;
          }>
        | {
              Success: boolean;
              Message: string;
          };
    RecordClassificationCorrection?: (request: {
        MessageID: string;
        Sender: string;
        OriginalCategory: ClassificationCategory;
        CorrectedCategory: ClassificationCategory;
    }) =>
        | Promise<{
              Success: boolean;
              Message: string;
          }>
        | {
              Success: boolean;
              Message: string;
          };
    GetBlocklistSuggestions?: () =>
        | Promise<
              Array<{
                  Kind: BlocklistKind;
                  Pattern: string;
                  Count: number;
                  LastSeenAt: string;
                  Description: string;
              }>
          >
        | Array<{
              Kind: BlocklistKind;
              Pattern: string;
              Count: number;
              LastSeenAt: string;
              Description: string;
          }>;
    RecordClassificationRun?: (request: {
        Messages: Array<{
            ID: string;
            ThreadID?: string;
            From: string;
            Subject: string;
            Snippet: string;
            Unread: boolean;
        }>;
        Results: Array<{
            MessageID: string;
            Category: ClassificationCategory;
            Confidence: number;
            Reason: string;
            ReviewLevel: ClassificationReviewLevel;
            Source?: 'claude' | 'blocklist';
        }>;
    }) =>
        | Promise<{
              Success: boolean;
              Message: string;
          }>
        | {
              Success: boolean;
              Message: string;
          };
    ExportProcessedMailCSV?: () =>
        | Promise<{ Success: boolean; Message: string }>
        | { Success: boolean; Message: string };
    ExportProcessedMailJSON?: () =>
        | Promise<{ Success: boolean; Message: string }>
        | { Success: boolean; Message: string };
    ExportBlocklistJSON?: () =>
        | Promise<{ Success: boolean; Message: string }>
        | { Success: boolean; Message: string };
    ImportBlocklistJSON?: () =>
        | Promise<{ Success: boolean; Message: string }>
        | { Success: boolean; Message: string };
    ExportImportantSummaryCSV?: () =>
        | Promise<{ Success: boolean; Message: string }>
        | { Success: boolean; Message: string };
    ExportImportantSummaryPDF?: () =>
        | Promise<{ Success: boolean; Message: string }>
        | { Success: boolean; Message: string };
    ExportDailyLogsCSV?: () =>
        | Promise<{ Success: boolean; Message: string }>
        | { Success: boolean; Message: string };
    ExportDailyLogsJSON?: () =>
        | Promise<{ Success: boolean; Message: string }>
        | { Success: boolean; Message: string };
};

declare global {
    interface Window {
        go?: {
            main?: {
                App?: WailsAppApi;
            };
        };
        runtime?: {
            EventsOn?: (eventName: string, callback: (...payload: unknown[]) => void) => void;
            EventsOff?: (eventName: string) => void;
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

export const defaultSchedulerSettings: SchedulerSettings = {
    classificationIntervalMinutes: 24 * 60,
    blocklistIntervalMinutes: 24 * 60,
    knownBlockIntervalMinutes: 30,
    notificationsEnabled: true,
};

const schedulerNotificationEventName = 'scheduler:notification';

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

export async function loadSchedulerSettings(): Promise<SchedulerSettings> {
    const appApi = window.go?.main?.App;
    const result = appApi?.GetSchedulerSettings?.();
    const raw = result ? await result : null;

    if (!raw) {
        return defaultSchedulerSettings;
    }

    return {
        classificationIntervalMinutes:
            raw.ClassificationIntervalMinutes ?? defaultSchedulerSettings.classificationIntervalMinutes,
        blocklistIntervalMinutes:
            raw.BlocklistIntervalMinutes ?? defaultSchedulerSettings.blocklistIntervalMinutes,
        knownBlockIntervalMinutes:
            raw.KnownBlockIntervalMinutes ?? defaultSchedulerSettings.knownBlockIntervalMinutes,
        notificationsEnabled: raw.NotificationsEnabled ?? defaultSchedulerSettings.notificationsEnabled,
    };
}

export async function updateSchedulerSettings(
    request: UpdateSchedulerSettingsRequest,
): Promise<OperationResult> {
    const appApi = window.go?.main?.App;
    const result = appApi?.UpdateSchedulerSettings?.({
        ClassificationIntervalMinutes: request.classificationIntervalMinutes,
        BlocklistIntervalMinutes: request.blocklistIntervalMinutes,
        KnownBlockIntervalMinutes: request.knownBlockIntervalMinutes,
        NotificationsEnabled: request.notificationsEnabled,
    });

    if (!result) {
        throw new Error('自動実行設定更新 API がまだ公開されていません。');
    }

    const raw = await result;
    return {
        success: raw.Success,
        message: raw.Message,
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
            source: item.Source ?? 'claude',
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

export async function executeGmailActions(
    request: ExecuteGmailActionsRequest,
): Promise<ExecuteGmailActionsResult> {
    const appApi = window.go?.main?.App;
    const result = appApi?.ExecuteGmailActions?.({
        Confirmed: request.confirmed,
        Decisions: request.decisions.map((item) => ({
            MessageID: item.messageID,
            Category: item.category,
            ReviewLevel: item.reviewLevel,
        })),
        Metadata: request.metadata.map((item) => ({
            MessageID: item.messageID,
            ThreadID: item.threadID,
            From: item.from,
            Subject: item.subject,
            Category: item.category,
            Confidence: item.confidence,
            ReviewLevel: item.reviewLevel,
            Source: item.source,
        })),
    });

    if (!result) {
        throw new Error('Gmail アクション実行 API がまだ公開されていません。');
    }

    const raw = await result;
    return {
        success: raw.Success,
        message: raw.Message,
        processedCount: raw.ProcessedCount ?? 0,
        successCount: raw.SuccessCount ?? 0,
        failureCount: raw.FailureCount ?? 0,
        deletedCount: raw.DeletedCount ?? 0,
        archivedCount: raw.ArchivedCount ?? 0,
        markedReadCount: raw.MarkedReadCount ?? 0,
        labeledCount: raw.LabeledCount ?? 0,
        createdLabels: raw.CreatedLabels ?? [],
        failures: (raw.Failures ?? []).map((failure) => ({
            messageID: failure.MessageID,
            action: failure.Action,
            error: failure.Error,
        })),
        tokenRefreshed: raw.TokenRefreshed ?? false,
    };
}

export async function loadBlocklistEntries(): Promise<BlocklistEntry[]> {
    const appApi = window.go?.main?.App;
    const result = appApi?.GetBlocklistEntries?.();

    if (!result) {
        throw new Error('ブロックリスト取得 API がまだ公開されていません。');
    }

    const raw = await result;
    const items = Array.isArray(raw) ? raw : [];
    return items.map((item) => ({
        id: item.ID,
        kind: item.Kind,
        pattern: item.Pattern,
        note: item.Note,
        createdAt: item.CreatedAt,
        updatedAt: item.UpdatedAt,
    }));
}

export async function upsertBlocklistEntry(
    request: UpsertBlocklistEntryRequest,
): Promise<BlocklistOperationResult> {
    const appApi = window.go?.main?.App;
    const result = appApi?.UpsertBlocklistEntry?.({
        Kind: request.kind,
        Pattern: request.pattern,
        Note: request.note,
    });

    if (!result) {
        throw new Error('ブロックリスト保存 API がまだ公開されていません。');
    }

    const raw = await result;
    return {
        success: raw.Success,
        message: raw.Message,
    };
}

export async function deleteBlocklistEntry(id: number): Promise<BlocklistOperationResult> {
    const appApi = window.go?.main?.App;
    const result = appApi?.DeleteBlocklistEntry?.(id);

    if (!result) {
        throw new Error('ブロックリスト削除 API がまだ公開されていません。');
    }

    const raw = await result;
    return {
        success: raw.Success,
        message: raw.Message,
    };
}

export async function recordClassificationCorrection(
    correction: ClassificationCorrection,
): Promise<BlocklistOperationResult> {
    const appApi = window.go?.main?.App;
    const result = appApi?.RecordClassificationCorrection?.({
        MessageID: correction.messageID,
        Sender: correction.sender,
        OriginalCategory: correction.originalCategory,
        CorrectedCategory: correction.correctedCategory,
    });

    if (!result) {
        throw new Error('分類修正履歴 API がまだ公開されていません。');
    }

    const raw = await result;
    return {
        success: raw.Success,
        message: raw.Message,
    };
}

export async function loadBlocklistSuggestions(): Promise<BlocklistSuggestion[]> {
    const appApi = window.go?.main?.App;
    const result = appApi?.GetBlocklistSuggestions?.();

    if (!result) {
        throw new Error('ブロック提案取得 API がまだ公開されていません。');
    }

    const raw = await result;
    const items = Array.isArray(raw) ? raw : [];
    return items.map((item) => ({
        kind: item.Kind,
        pattern: item.Pattern,
        count: item.Count,
        lastSeenAt: item.LastSeenAt,
        description: item.Description,
    }));
}

export async function recordClassificationRun(
    request: RecordClassificationRunRequest,
): Promise<OperationResult> {
    const appApi = window.go?.main?.App;
    const result = appApi?.RecordClassificationRun?.({
        Messages: request.messages.map((message) => ({
            ID: message.id,
            ThreadID: message.threadID,
            From: message.from,
            Subject: message.subject,
            Snippet: message.snippet,
            Unread: message.unread,
        })),
        Results: request.results.map((item) => ({
            MessageID: item.messageID,
            Category: item.category,
            Confidence: item.confidence,
            Reason: item.reason,
            ReviewLevel: item.reviewLevel,
            Source: item.source,
        })),
    });

    if (!result) {
        throw new Error('分類ログ保存 API がまだ公開されていません。');
    }

    const raw = await result;
    return {
        success: raw.Success,
        message: raw.Message,
    };
}

function parseSchedulerNotificationPayload(payload: unknown): SchedulerNotification | null {
    const value =
        Array.isArray(payload) && payload.length === 1
            ? payload[0]
            : payload;

    if (!value || typeof value !== 'object') {
        return null;
    }

    const item = value as Partial<{
        Title: string;
        Body: string;
        Level: string;
        JobID: string;
        At: string;
    }>;

    const level =
        item.Level === 'warning' || item.Level === 'error'
            ? item.Level
            : 'info';

    return {
        title: item.Title ?? 'Mairu 自動実行',
        body: item.Body ?? '',
        level,
        jobID: item.JobID ?? '',
        at: item.At ?? new Date().toISOString(),
    };
}

export function subscribeSchedulerNotifications(
    listener: (notification: SchedulerNotification) => void,
): () => void {
    const runtimeApi = window.runtime;
    const eventsOn = runtimeApi?.EventsOn;
    const eventsOff = runtimeApi?.EventsOff;

    if (!eventsOn || !eventsOff) {
        return () => {};
    }

    const handler = (...payload: unknown[]) => {
        const parsed = parseSchedulerNotificationPayload(payload);
        if (parsed) {
            listener(parsed);
        }
    };
    eventsOn(schedulerNotificationEventName, handler);

    return () => {
        eventsOff(schedulerNotificationEventName);
    };
}

export function getNotificationPermissionStatus(): NotificationPermission | 'unsupported' {
    if (typeof Notification === 'undefined') {
        return 'unsupported';
    }
    return Notification.permission;
}

export async function requestNotificationPermission(): Promise<NotificationPermission | 'unsupported'> {
    if (typeof Notification === 'undefined') {
        return 'unsupported';
    }
    return Notification.requestPermission();
}

export function showSchedulerNotification(notification: SchedulerNotification): boolean {
    if (typeof Notification === 'undefined') {
        return false;
    }
    if (Notification.permission !== 'granted') {
        return false;
    }

    try {
        new Notification(notification.title, {
            body: notification.body,
            tag: `mairu-${notification.jobID}-${notification.level}`,
        });
        return true;
    } catch {
        return false;
    }
}

async function runExportOperation(
    operation: (() => Promise<{ Success: boolean; Message: string } | { Success: boolean; Message: string }> | { Success: boolean; Message: string } | undefined),
    missingMessage: string,
): Promise<OperationResult> {
    const result = operation?.();
    if (!result) {
        throw new Error(missingMessage);
    }

    const raw = await result;
    return {
        success: raw.Success,
        message: raw.Message,
    };
}

export async function exportProcessedMailCSV(): Promise<OperationResult> {
    const appApi = window.go?.main?.App;
    return runExportOperation(
        () => appApi?.ExportProcessedMailCSV?.(),
        '処理済みメール CSV エクスポート API がまだ公開されていません。',
    );
}

export async function exportProcessedMailJSON(): Promise<OperationResult> {
    const appApi = window.go?.main?.App;
    return runExportOperation(
        () => appApi?.ExportProcessedMailJSON?.(),
        '処理済みメール JSON エクスポート API がまだ公開されていません。',
    );
}

export async function exportBlocklistJSON(): Promise<OperationResult> {
    const appApi = window.go?.main?.App;
    return runExportOperation(
        () => appApi?.ExportBlocklistJSON?.(),
        'blocklist JSON エクスポート API がまだ公開されていません。',
    );
}

export async function importBlocklistJSON(): Promise<OperationResult> {
    const appApi = window.go?.main?.App;
    return runExportOperation(
        () => appApi?.ImportBlocklistJSON?.(),
        'blocklist JSON インポート API がまだ公開されていません。',
    );
}

export async function exportImportantSummaryCSV(): Promise<OperationResult> {
    const appApi = window.go?.main?.App;
    return runExportOperation(
        () => appApi?.ExportImportantSummaryCSV?.(),
        '重要メール CSV エクスポート API がまだ公開されていません。',
    );
}

export async function exportImportantSummaryPDF(): Promise<OperationResult> {
    const appApi = window.go?.main?.App;
    return runExportOperation(
        () => appApi?.ExportImportantSummaryPDF?.(),
        '重要メール PDF エクスポート API がまだ公開されていません。',
    );
}

export async function exportDailyLogsCSV(): Promise<OperationResult> {
    const appApi = window.go?.main?.App;
    return runExportOperation(
        () => appApi?.ExportDailyLogsCSV?.(),
        '日別ログ CSV エクスポート API がまだ公開されていません。',
    );
}

export async function exportDailyLogsJSON(): Promise<OperationResult> {
    const appApi = window.go?.main?.App;
    return runExportOperation(
        () => appApi?.ExportDailyLogsJSON?.(),
        '日別ログ JSON エクスポート API がまだ公開されていません。',
    );
}

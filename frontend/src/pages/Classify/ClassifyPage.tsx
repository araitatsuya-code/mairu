import './ClassifyPage.css';

import { useEffect, useMemo, useRef, useState } from 'react';

import {
    classifyEmails,
    fetchClassificationMessages,
    fetchGmailMessageDetail,
    listGmailLabels,
    type ClassificationCategory,
    type ClassificationResponse,
    type ClassificationReviewLevel,
    type EmailSummary,
    executeGmailActions,
    type GmailLabel,
    type GmailMessageDetail,
    recordClassificationCorrection,
    recordClassificationRun,
    type ExecuteGmailActionsResult,
    type RuntimeStatus,
} from '../../lib/runtime';

type ClassifyPageProps = {
    status: RuntimeStatus;
};

type ReviewFilter = 'all' | ClassificationReviewLevel;

type ClassifiedRow = {
    result: ClassificationResponse['results'][number];
    message: EmailSummary | undefined;
};

type ActionExecutionHistory = {
    at: string;
    selectedCount: number;
    result: ExecuteGmailActionsResult;
};

type ClassificationDataMode = 'live' | 'sample' | 'mock';

const maxFetchCount = 50;
const defaultFetchCount = 50;
const defaultLabelID = 'INBOX';

const sampleSenders = [
    'alerts@bank.example',
    'newsletter@service.example',
    'hr@company.example',
    'support@shop.example',
    'noreply@community.example',
    'team@project.example',
    'campaign@travel.example',
    'billing@saas.example',
    'updates@product.example',
    'contact@client.example',
];

const sampleSubjects = [
    '確認依頼: 今週の対応タスク',
    '新着ニュースレターのお知らせ',
    '請求明細のご案内',
    'イベント参加のご招待',
    'アカウント通知: セキュリティ更新',
    'プロジェクト進捗レビュー',
    'キャンペーン情報のお知らせ',
    '返信お願いします: 仕様確認',
    '未読メールの優先確認',
    '不要メールの自動整理候補',
];

const sampleSnippets = [
    '本日中に確認が必要な内容です。返信予定があれば共有してください。',
    '毎週配信しているニュースレターです。要点のみ確認できれば十分です。',
    '支払い状況と利用明細をまとめました。金額に誤りがないか確認してください。',
    '次回イベントの案内です。参加可否の回答期限は明日です。',
    'セキュリティに関する重要なお知らせです。設定変更の反映が必要です。',
    '担当チームから進捗報告が届きました。レビューコメントをお願いします。',
    '期間限定キャンペーン情報です。不要なら自動アーカイブ候補です。',
    '顧客からの返信待ち案件です。優先度が高いため未読維持を推奨します。',
    '承認待ちの依頼が含まれています。本文を確認して対応方針を決めてください。',
    '分類精度が低い可能性があります。理由を確認して手動判断してください。',
];

const categoryOrder: ClassificationCategory[] = [
    'important',
    'newsletter',
    'junk',
    'archive',
    'unread_priority',
];

const confidencePattern = [0.96, 0.82, 0.63, 0.42, 0.91, 0.74, 0.55, 0.37];

const categoryOptions: Array<{ value: ClassificationCategory; label: string }> = [
    { value: 'important', label: '重要' },
    { value: 'newsletter', label: 'ニュースレター' },
    { value: 'junk', label: '不要' },
    { value: 'archive', label: 'アーカイブ' },
    { value: 'unread_priority', label: '未読優先' },
];

const reviewFilterOptions: Array<{ value: ReviewFilter; label: string }> = [
    { value: 'all', label: 'すべて' },
    { value: 'auto_apply', label: '自動実行' },
    { value: 'review', label: '承認待ち' },
    { value: 'review_with_reason', label: '要理由確認' },
    { value: 'hold', label: '保留' },
];

function buildSampleMessages(count: number): EmailSummary[] {
    return Array.from({ length: count }, (_, index) => {
        const sender = sampleSenders[index % sampleSenders.length];
        const subjectBase = sampleSubjects[index % sampleSubjects.length];
        const snippetBase = sampleSnippets[index % sampleSnippets.length];
        const unread = index % 3 !== 0;

        return {
            id: `sample-${String(index + 1).padStart(2, '0')}`,
            threadID: `thread-${String(Math.floor(index / 2) + 1).padStart(2, '0')}`,
            from: sender,
            subject: `${subjectBase} #${index + 1}`,
            snippet: `${snippetBase} (サンプル ${index + 1})`,
            unread,
        };
    });
}

function reviewLevelForConfidence(confidence: number): ClassificationReviewLevel {
    if (confidence >= 0.9) {
        return 'auto_apply';
    }
    if (confidence >= 0.7) {
        return 'review';
    }
    if (confidence >= 0.5) {
        return 'review_with_reason';
    }
    return 'hold';
}

function buildMockResults(messages: EmailSummary[]): ClassificationResponse['results'] {
    return messages.map((message, index) => {
        const category = categoryOrder[index % categoryOrder.length];
        const confidence = confidencePattern[index % confidencePattern.length];
        return {
            messageID: message.id,
            category,
            confidence,
            reviewLevel: reviewLevelForConfidence(confidence),
            reason: `件名と差出人の傾向から ${category} と判定しました。`,
            source: 'claude',
        };
    });
}

function categoryLabel(category: ClassificationCategory): string {
    switch (category) {
    case 'important':
        return '重要';
    case 'newsletter':
        return 'ニュースレター';
    case 'junk':
        return '不要';
    case 'archive':
        return 'アーカイブ';
    case 'unread_priority':
        return '未読優先';
    default:
        return category;
    }
}

function reviewLabel(reviewLevel: ClassificationReviewLevel): string {
    switch (reviewLevel) {
    case 'auto_apply':
        return '自動実行';
    case 'review':
        return '承認待ち';
    case 'review_with_reason':
        return '要理由確認';
    case 'hold':
        return '保留';
    default:
        return reviewLevel;
    }
}

function recommendedAction(
    category: ClassificationCategory,
    reviewLevel: ClassificationReviewLevel,
): string {
    if (reviewLevel === 'hold') {
        return '実行保留。本文確認後に手動で判断';
    }

    switch (category) {
    case 'important':
        return '重要ラベルを付与して未読維持';
    case 'newsletter':
        return 'ニュースレターへ分類して既読化';
    case 'junk':
        return '削除候補として確認ダイアログへ';
    case 'archive':
        return 'アーカイブ処理へ送る';
    case 'unread_priority':
        return '未読優先キューへ移動';
    default:
        return '手動確認';
    }
}

function formatDateTime(value: string | null): string {
    if (!value) {
        return '未実行';
    }

    const parsed = new Date(value);
    if (Number.isNaN(parsed.getTime())) {
        return value;
    }

    return new Intl.DateTimeFormat('ja-JP', {
        dateStyle: 'medium',
        timeStyle: 'short',
    }).format(parsed);
}

function actionKindLabel(action: 'label' | 'archive' | 'delete' | 'mark_read'): string {
    switch (action) {
    case 'label':
        return 'ラベル付与';
    case 'archive':
        return 'アーカイブ';
    case 'delete':
        return '削除';
    case 'mark_read':
        return '既読化';
    default:
        return action;
    }
}

function normalizeFetchCount(value: number): number {
    if (!Number.isFinite(value)) {
        return defaultFetchCount;
    }
    const rounded = Math.round(value);
    if (rounded < 1) {
        return 1;
    }
    if (rounded > maxFetchCount) {
        return maxFetchCount;
    }
    return rounded;
}

function normalizeFetchCountInput(value: string): string {
    return String(normalizeFetchCount(Number(value)));
}

function extractErrorMessage(cause: unknown, fallback: string): string {
    if (cause instanceof Error) {
        const message = cause.message.trim();
        if (message !== '') {
            return message;
        }
    }
    if (typeof cause === 'string') {
        const message = cause.trim();
        if (message !== '') {
            return message;
        }
    }
    if (cause && typeof cause === 'object') {
        const withMessage = cause as { message?: unknown; error?: unknown };
        if (typeof withMessage.message === 'string' && withMessage.message.trim() !== '') {
            return withMessage.message.trim();
        }
        if (typeof withMessage.error === 'string' && withMessage.error.trim() !== '') {
            return withMessage.error.trim();
        }
        try {
            const serialized = JSON.stringify(cause);
            if (serialized && serialized !== '{}') {
                return serialized;
            }
        } catch {
            // ignore serialization errors and use fallback
        }
    }
    return fallback;
}

function buildSampleMessageDetail(message: EmailSummary): GmailMessageDetail {
    return {
        id: message.id,
        threadID: message.threadID,
        from: message.from,
        to: '',
        subject: message.subject,
        date: '',
        snippet: message.snippet,
        labelIDs: message.unread ? ['INBOX', 'UNREAD'] : ['INBOX'],
        unread: message.unread,
        bodyText: message.snippet,
        bodyHTML: '',
        headers: [
            { name: 'From', value: message.from },
            { name: 'Subject', value: message.subject },
        ],
    };
}

export function ClassifyPage({ status }: ClassifyPageProps) {
    const [messages, setMessages] = useState<EmailSummary[]>([]);
    const [results, setResults] = useState<ClassificationResponse['results']>([]);
    const [filter, setFilter] = useState<ReviewFilter>('all');
    const [selectedForApproval, setSelectedForApproval] = useState<Record<string, boolean>>({});
    const [correctedCategories, setCorrectedCategories] = useState<
        Record<string, ClassificationCategory>
    >({});
    const [classifyModel, setClassifyModel] = useState('');
    const [fetchQuery, setFetchQuery] = useState('');
    const [fetchCountInput, setFetchCountInput] = useState(String(defaultFetchCount));
    const [selectedLabelIDs, setSelectedLabelIDs] = useState<string[]>([defaultLabelID]);
    const [availableLabels, setAvailableLabels] = useState<GmailLabel[]>([]);
    const [labelsPending, setLabelsPending] = useState(false);
    const [labelsError, setLabelsError] = useState<string | null>(null);
    const [labelsMessage, setLabelsMessage] = useState<string | null>(null);
    const [nextPageToken, setNextPageToken] = useState('');
    const [fetchPending, setFetchPending] = useState(false);
    const [pending, setPending] = useState(false);
    const [error, setError] = useState<string | null>(null);
    const [fetchError, setFetchError] = useState<string | null>(null);
    const [actionPending, setActionPending] = useState(false);
    const [actionConfirmOpen, setActionConfirmOpen] = useState(false);
    const [actionError, setActionError] = useState<string | null>(null);
    const [correctionMessage, setCorrectionMessage] = useState<string | null>(null);
    const [correctionError, setCorrectionError] = useState<string | null>(null);
    const [correctionPendingByID, setCorrectionPendingByID] = useState<Record<string, boolean>>({});
    const [lastActionResult, setLastActionResult] = useState<ExecuteGmailActionsResult | null>(null);
    const [actionHistory, setActionHistory] = useState<ActionExecutionHistory[]>([]);
    const [fetchMessage, setFetchMessage] = useState<string | null>(null);
    const [loggingMessage, setLoggingMessage] = useState<string | null>(null);
    const [selectedMessageID, setSelectedMessageID] = useState<string | null>(null);
    const [selectedMessageDetail, setSelectedMessageDetail] = useState<GmailMessageDetail | null>(null);
    const [messageDetailPending, setMessageDetailPending] = useState(false);
    const [messageDetailError, setMessageDetailError] = useState<string | null>(null);
    const [classificationDataMode, setClassificationDataMode] = useState<ClassificationDataMode>('live');
    const [lastFetchedAt, setLastFetchedAt] = useState<string | null>(null);
    const [lastClassifiedAt, setLastClassifiedAt] = useState<string | null>(null);
    const messageDetailRequestIDRef = useRef(0);
    const isLiveDataMode = classificationDataMode === 'live';

    const messageByID = useMemo(() => {
        return new Map(messages.map((message) => [message.id, message]));
    }, [messages]);

    const rows = useMemo<ClassifiedRow[]>(() => {
        return results.map((result) => ({
            result,
            message: messageByID.get(result.messageID),
        }));
    }, [messageByID, results]);

    const resultByMessageID = useMemo(() => {
        return new Map(results.map((result) => [result.messageID, result]));
    }, [results]);

    const filteredRows = useMemo(() => {
        if (filter === 'all') {
            return rows;
        }
        return rows.filter((row) => row.result.reviewLevel === filter);
    }, [filter, rows]);

    const reviewCounts = useMemo(() => {
        const initial: Record<ClassificationReviewLevel, number> = {
            auto_apply: 0,
            review: 0,
            review_with_reason: 0,
            hold: 0,
        };

        for (const row of rows) {
            initial[row.result.reviewLevel] += 1;
        }

        return initial;
    }, [rows]);

    const approvalCandidateCount = useMemo(() => {
        return rows.filter((row) => row.result.reviewLevel !== 'hold').length;
    }, [rows]);

    const selectedApprovalCount = useMemo(() => {
        return rows.filter((row) => selectedForApproval[row.result.messageID]).length;
    }, [rows, selectedForApproval]);

    const selectedDecisions = useMemo(() => {
        return rows
            .filter((row) => selectedForApproval[row.result.messageID])
            .map((row) => ({
                messageID: row.result.messageID,
                category: correctedCategories[row.result.messageID] ?? row.result.category,
                reviewLevel: row.result.reviewLevel,
            }));
    }, [correctedCategories, rows, selectedForApproval]);

    const selectedDeleteCount = useMemo(() => {
        return selectedDecisions.filter((item) => item.category === 'junk').length;
    }, [selectedDecisions]);

    function resetApprovalSelection() {
        setSelectedForApproval({});
        setActionConfirmOpen(false);
    }

    function resetCorrections() {
        setCorrectedCategories({});
        setCorrectionMessage(null);
        setCorrectionError(null);
        setCorrectionPendingByID({});
    }

    function clearMessageDetail() {
        messageDetailRequestIDRef.current += 1;
        setSelectedMessageID(null);
        setSelectedMessageDetail(null);
        setMessageDetailError(null);
        setMessageDetailPending(false);
    }

    async function persistClassificationLogs(
        nextMessages: EmailSummary[],
        nextResults: ClassificationResponse['results'],
    ) {
        if (!status.databaseReady) {
            setLoggingMessage('SQLite 未初期化のため分類ログ保存はスキップしました。');
            return;
        }

        const result = await recordClassificationRun({
            messages: nextMessages,
            results: nextResults,
        });
        if (!result.success) {
            throw new Error(result.message);
        }
        setLoggingMessage(result.message);
    }

    function ensureSelectableLabelIDs(nextLabels: GmailLabel[], previousSelected: string[]): string[] {
        const availableSet = new Set(nextLabels.map((label) => label.id));
        const filtered = previousSelected.filter((id) => availableSet.has(id));
        if (filtered.length > 0) {
            return filtered;
        }
        if (availableSet.has(defaultLabelID)) {
            return [defaultLabelID];
        }
        if (nextLabels.length > 0) {
            return [nextLabels[0].id];
        }
        return [];
    }

    async function handleLoadLabels() {
        if (!status.gmailConnected) {
            setLabelsError('Gmail 接続未確認のためラベル一覧を取得できません。');
            return;
        }

        setLabelsPending(true);
        setLabelsError(null);
        setLabelsMessage(null);

        try {
            const response = await listGmailLabels();
            setAvailableLabels(response.labels);
            setSelectedLabelIDs((previous) => ensureSelectableLabelIDs(response.labels, previous));
            setNextPageToken('');
            if (response.labels.length === 0) {
                setLabelsMessage('取得可能なラベルが見つかりませんでした。');
            } else {
                const refreshedText = response.tokenRefreshed
                    ? ' 実行前に Google トークンを更新しました。'
                    : '';
                setLabelsMessage(`ラベル候補 ${response.labels.length} 件を同期しました。${refreshedText}`);
            }
        } catch (cause) {
            setLabelsError(extractErrorMessage(cause, 'Gmail ラベル一覧の取得に失敗しました。'));
        } finally {
            setLabelsPending(false);
        }
    }

    useEffect(() => {
        if (!status.gmailConnected) {
            setAvailableLabels([]);
            setSelectedLabelIDs([]);
            setLabelsError(null);
            setLabelsMessage(null);
            return;
        }

        void handleLoadLabels();
    // eslint-disable-next-line react-hooks/exhaustive-deps
    }, [status.gmailConnected]);

    function handleLabelSelectionChange(event: React.ChangeEvent<HTMLSelectElement>) {
        const next = Array.from(event.target.selectedOptions).map((option) => option.value);
        setSelectedLabelIDs(next);
        setNextPageToken('');
    }

    function handleQueryChange(event: React.ChangeEvent<HTMLInputElement>) {
        setFetchQuery(event.target.value);
        setNextPageToken('');
    }

    function handleCountChange(event: React.ChangeEvent<HTMLInputElement>) {
        const raw = event.target.value;
        if (raw === '' || /^[0-9]+$/.test(raw)) {
            setFetchCountInput(raw);
            setNextPageToken('');
        }
    }

    function handleCountBlur() {
        setFetchCountInput((previous) => normalizeFetchCountInput(previous));
    }

    async function handleFetchMessages(pageToken = '') {
        if (!status.gmailConnected) {
            setFetchError('Gmail 接続未確認のため実メール取得できません。Settings で接続確認を行ってください。');
            return;
        }

        const normalizedCount = normalizeFetchCount(Number(fetchCountInput));
        setFetchCountInput(String(normalizedCount));
        const labelIDs = selectedLabelIDs;

        setFetchPending(true);
        setFetchError(null);
        setFetchMessage(null);
        setError(null);

        try {
            const response = await fetchClassificationMessages({
                query: fetchQuery.trim(),
                maxResults: normalizedCount,
                labelIDs,
                pageToken,
            });

            setMessages(response.messages);
            setResults([]);
            setClassificationDataMode('live');
            setNextPageToken(response.nextPageToken);
            setLastFetchedAt(new Date().toISOString());
            setLastClassifiedAt(null);
            setActionError(null);
            setLastActionResult(null);
            setLoggingMessage(null);
            resetApprovalSelection();
            resetCorrections();
            clearMessageDetail();

            if (response.messages.length === 0) {
                setFetchMessage('指定条件に一致するメールはありませんでした。条件を変更して再取得してください。');
            } else {
                const queryText = fetchQuery.trim() || '指定なし';
                const labelText = labelIDs.length > 0 ? labelIDs.join(', ') : '指定なし';
                const pageText = response.nextPageToken ? ' 次ページがあります。' : ' 最終ページです。';
                const refreshedText = response.tokenRefreshed
                    ? ' 実行前に Google トークンを更新しました。'
                    : '';
                setFetchMessage(
                    `実メール ${response.messages.length} 件を取得しました (query: ${queryText} / labels: ${labelText})。${pageText}${refreshedText}`,
                );
            }
        } catch (cause) {
            setFetchError(extractErrorMessage(cause, '実メール取得に失敗しました。'));
        } finally {
            setFetchPending(false);
        }
    }

    async function handleGenerateSample() {
        const generated = buildSampleMessages(defaultFetchCount);
        const nextResults = buildMockResults(generated);
        setMessages(generated);
        setResults(nextResults);
        setClassificationDataMode('sample');
        setFetchQuery('');
        setSelectedLabelIDs([defaultLabelID]);
        setFetchCountInput(String(defaultFetchCount));
        setNextPageToken('');
        setFetchMessage('開発補助のサンプル 50 件を読み込みました。');
        setFetchError(null);
        setError(null);
        setActionError(null);
        setLastActionResult(null);
        setLastFetchedAt(new Date().toISOString());
        setLastClassifiedAt(new Date().toISOString());
        resetApprovalSelection();
        resetCorrections();
        clearMessageDetail();
        setLoggingMessage('開発補助のサンプル結果のため分類ログ保存はスキップしました。');
    }

    async function handleShowMockResults() {
        if (messages.length === 0) {
            setError('モック分類の対象メールがありません。先に実メール取得またはサンプル生成を実行してください。');
            return;
        }

        const nextResults = buildMockResults(messages);
        setResults(nextResults);
        setClassificationDataMode('mock');
        setError(null);
        setActionError(null);
        setLastActionResult(null);
        setLastClassifiedAt(new Date().toISOString());
        resetApprovalSelection();
        resetCorrections();
        setLoggingMessage('開発補助のモック結果のため分類ログ保存はスキップしました。');
    }

    async function handleClassifyWithClaude() {
        if (messages.length === 0) {
            setError('分類対象の実メールがありません。先に実メール取得を実行してください。');
            return;
        }

        setPending(true);
        setError(null);

        try {
            const response = await classifyEmails({
                model: classifyModel.trim() || undefined,
                messages,
            });
            if (response.results.length === 0) {
                setError('分類結果が空でした。API キー設定と入力データを確認してください。');
                return;
            }

            const includesSampleMessages =
                messages.some((message) => message.id.startsWith('sample-')) ||
                response.results.some((result) => result.messageID.startsWith('sample-'));

            setResults(response.results);
            setClassificationDataMode(includesSampleMessages ? 'sample' : 'live');
            setLastClassifiedAt(new Date().toISOString());
            setActionError(null);
            setLastActionResult(null);
            resetApprovalSelection();
            resetCorrections();
            if (includesSampleMessages) {
                setLoggingMessage('サンプル結果のため分類ログ保存はスキップしました。');
            } else {
                await persistClassificationLogs(messages, response.results);
            }
        } catch (cause) {
            setError(extractErrorMessage(cause, 'Claude 分類に失敗しました。'));
        } finally {
            setPending(false);
        }
    }

    function handleToggleApproval(messageID: string) {
        setSelectedForApproval((previous) => ({
            ...previous,
            [messageID]: !previous[messageID],
        }));
    }

    function handleSelectAllExecutableRows() {
        const next: Record<string, boolean> = {};
        for (const row of rows) {
            if (row.result.reviewLevel === 'hold') {
                continue;
            }
            next[row.result.messageID] = true;
        }
        setSelectedForApproval(next);
    }

    async function handleOpenMessageDetail(messageID: string) {
        const summary = messageByID.get(messageID);
        if (!summary) {
            return;
        }

        const requestID = messageDetailRequestIDRef.current + 1;
        messageDetailRequestIDRef.current = requestID;
        setSelectedMessageID(messageID);
        setMessageDetailError(null);
        setSelectedMessageDetail(null);

        if (messageID.startsWith('sample-')) {
            setMessageDetailPending(false);
            setSelectedMessageDetail(buildSampleMessageDetail(summary));
            return;
        }

        setMessageDetailPending(true);
        try {
            const detail = await fetchGmailMessageDetail(messageID);
            if (messageDetailRequestIDRef.current !== requestID) {
                return;
            }
            setSelectedMessageDetail(detail);
        } catch (cause) {
            if (messageDetailRequestIDRef.current !== requestID) {
                return;
            }
            setMessageDetailError(extractErrorMessage(cause, 'メール詳細の取得に失敗しました。'));
        } finally {
            if (messageDetailRequestIDRef.current === requestID) {
                setMessageDetailPending(false);
            }
        }
    }

    async function handleCategoryCorrection(row: ClassifiedRow, nextCategory: ClassificationCategory) {
        if (!isLiveDataMode) {
            setCorrectionError('サンプル/モック結果では分類修正を保存できません。');
            setCorrectionMessage(null);
            return;
        }

        const messageID = row.result.messageID;
        setCorrectedCategories((previous) => ({
            ...previous,
            [messageID]: nextCategory,
        }));

        if (nextCategory === row.result.category) {
            setCorrectionError(null);
            setCorrectionMessage(null);
            return;
        }

        if (!row.message) {
            setCorrectionError('修正履歴の保存対象メール情報が見つかりません。');
            return;
        }

        if (!status.databaseReady) {
            setCorrectionError('SQLite 未初期化のため修正履歴を保存できません。');
            return;
        }

        setCorrectionPendingByID((previous) => ({
            ...previous,
            [messageID]: true,
        }));
        setCorrectionError(null);
        setCorrectionMessage(null);

        try {
            const result = await recordClassificationCorrection({
                messageID,
                sender: row.message.from,
                originalCategory: row.result.category,
                correctedCategory: nextCategory,
            });
            if (!result.success) {
                setCorrectionError(result.message);
                return;
            }
            setCorrectionMessage(
                `${messageID} の修正を保存しました。3 回以上の同一傾向で Blocklist 提案に表示されます。`,
            );
        } catch (cause) {
            setCorrectionError(extractErrorMessage(cause, '修正履歴の保存に失敗しました。'));
        } finally {
            setCorrectionPendingByID((previous) => ({
                ...previous,
                [messageID]: false,
            }));
        }
    }

    async function handleExecuteSelectedActions() {
        if (!isLiveDataMode) {
            setActionError('サンプル/モック結果は Gmail に適用できません。実メールを取得して Claude 分類を実行してください。');
            return;
        }
        if (selectedDecisions.length === 0) {
            setActionError('実行対象を選択してください。');
            return;
        }
        setActionError(null);
        setActionConfirmOpen(true);
    }

    async function handleConfirmExecuteSelectedActions() {
        if (!isLiveDataMode) {
            setActionError('サンプル/モック結果は Gmail に適用できません。');
            setActionConfirmOpen(false);
            return;
        }
        if (selectedDecisions.length === 0) {
            setActionError('実行対象を選択してください。');
            setActionConfirmOpen(false);
            return;
        }

        setActionConfirmOpen(false);
        setActionPending(true);
        setActionError(null);

        try {
            const result = await executeGmailActions({
                confirmed: true,
                decisions: selectedDecisions,
                metadata: rows
                    .filter((row) => selectedForApproval[row.result.messageID])
                    .map((row) => ({
                        messageID: row.result.messageID,
                        threadID: row.message?.threadID ?? '',
                        from: row.message?.from ?? '',
                        subject: row.message?.subject ?? '',
                        category: correctedCategories[row.result.messageID] ?? row.result.category,
                        confidence: row.result.confidence,
                        reviewLevel: row.result.reviewLevel,
                        source: row.result.source,
                    })),
            });
            setLastActionResult(result);
            setActionHistory((previous) => [
                {
                    at: new Date().toISOString(),
                    selectedCount: selectedDecisions.length,
                    result,
                },
                ...previous,
            ]);
            resetApprovalSelection();

            if (!result.success) {
                setActionError(result.message);
            }
        } catch (cause) {
            setActionError(extractErrorMessage(cause, 'Gmail アクション実行に失敗しました。'));
        } finally {
            setActionPending(false);
        }
    }

    return (
        <div className="classify-page">
            <section className="classify-hero">
                <p className="classify-eyebrow">MAIRU-024 / #48</p>
                <h1>実メール分類フロー</h1>
                <p className="classify-lead">
                    Classify 画面内で「実メール取得 → Claude 分類 → 承認選択 → Gmail 反映」までを完結させます。
                    query・件数・ラベル条件を指定し、反映結果と失敗理由をこの画面で追跡できます。
                </p>
                <div className="classify-hero-status">
                    <span className={`hero-chip ${status.claudeConfigured ? 'ready' : 'pending'}`}>
                        Claude API: {status.claudeConfigured ? '設定済み' : '未設定'}
                    </span>
                    <span className={`hero-chip ${status.gmailConnected ? 'ready' : 'pending'}`}>
                        Gmail 接続: {status.gmailConnected ? '確認済み' : '未確認'}
                    </span>
                </div>
            </section>

            <section className="classify-controls" aria-label="実メール取得と分類実行">
                <div className="classify-control-row classify-fetch-row">
                    <label className="classify-field" htmlFor="fetch-query">
                        <span>Gmail クエリ (任意)</span>
                        <input
                            id="fetch-query"
                            type="text"
                            value={fetchQuery}
                            onChange={handleQueryChange}
                            placeholder="例: newer_than:7d -category:promotions"
                        />
                    </label>
                    <label className="classify-field classify-field-sm" htmlFor="fetch-count">
                        <span>取得件数 (1-50)</span>
                        <input
                            id="fetch-count"
                            type="number"
                            min={1}
                            max={maxFetchCount}
                            value={fetchCountInput}
                            onChange={handleCountChange}
                            onBlur={handleCountBlur}
                        />
                    </label>
                    <label className="classify-field" htmlFor="fetch-labels">
                        <span>対象ラベル (自動取得 / 複数選択)</span>
                        <select
                            id="fetch-labels"
                            className="classify-label-select"
                            multiple
                            size={6}
                            value={selectedLabelIDs}
                            onChange={handleLabelSelectionChange}
                            disabled={labelsPending || !status.gmailConnected || availableLabels.length === 0}
                        >
                            {availableLabels.map((label) => (
                                <option key={label.id} value={label.id}>
                                    {label.name} ({label.id}){label.type ? ` / ${label.type}` : ''}
                                </option>
                            ))}
                        </select>
                    </label>
                    <button
                        type="button"
                        className="classify-secondary-button"
                        onClick={() => {
                            void handleLoadLabels();
                        }}
                        disabled={labelsPending || fetchPending || pending || !status.gmailConnected}
                    >
                        {labelsPending ? 'ラベル同期中...' : 'ラベル候補を更新'}
                    </button>
                    <button
                        type="button"
                        className="classify-primary-button"
                        onClick={() => {
                            void handleFetchMessages();
                        }}
                        disabled={fetchPending || pending || actionPending || !status.gmailConnected}
                    >
                        {fetchPending ? '実メール取得中...' : '実メールを取得'}
                    </button>
                    <button
                        type="button"
                        className="classify-secondary-button"
                        onClick={() => {
                            void handleFetchMessages(nextPageToken);
                        }}
                        disabled={
                            fetchPending ||
                            pending ||
                            actionPending ||
                            !status.gmailConnected ||
                            nextPageToken === ''
                        }
                    >
                        次ページを取得
                    </button>
                </div>

                <div className="classify-control-row classify-model-row">
                    <label className="classify-field" htmlFor="classify-model">
                        <span>Claude モデル (任意)</span>
                        <input
                            id="classify-model"
                            type="text"
                            value={classifyModel}
                            onChange={(event) => {
                                setClassifyModel(event.target.value);
                            }}
                            placeholder="例: claude-3-5-sonnet-latest"
                        />
                    </label>
                    <button
                        type="button"
                        className="classify-primary-button"
                        onClick={() => {
                            void handleClassifyWithClaude();
                        }}
                        disabled={pending || fetchPending || !status.claudeConfigured || messages.length === 0}
                    >
                        {pending ? 'Claude 分類中...' : '取得メールを Claude で分類'}
                    </button>
                </div>

                <p className="classify-inline-note">
                    取得件数: {messages.length} 件 / 最終取得: {formatDateTime(lastFetchedAt)} / 最終分類: {formatDateTime(lastClassifiedAt)}
                </p>
                {!status.gmailConnected ? (
                    <p className="classify-inline-note">
                        Gmail 接続未確認のため実メール取得を無効化しています。Settings で接続確認を先に行ってください。
                    </p>
                ) : null}
                {!status.claudeConfigured ? (
                    <p className="classify-inline-note">
                        Claude API キー未設定のため、分類実行を無効化しています。Settings から設定できます。
                    </p>
                ) : null}
                <p className="classify-inline-note">
                    選択ラベル: {selectedLabelIDs.length > 0 ? selectedLabelIDs.join(', ') : '未選択'}
                </p>
                {!isLiveDataMode ? (
                    <p className="classify-inline-note">
                        現在は{classificationDataMode === 'sample' ? 'サンプル' : 'モック'}結果表示中のため、
                        分類修正保存と Gmail 反映を無効化しています。
                    </p>
                ) : null}
                {labelsMessage ? <p className="classify-inline-note">{labelsMessage}</p> : null}
                {labelsError ? <p className="classify-error-note">{labelsError}</p> : null}
                {fetchMessage ? <p className="classify-inline-note">{fetchMessage}</p> : null}
                {fetchError ? <p className="classify-error-note">{fetchError}</p> : null}
                {error ? <p className="classify-error-note">{error}</p> : null}
                {loggingMessage ? <p className="classify-inline-note">{loggingMessage}</p> : null}

                <details className="classify-devtools">
                    <summary>開発補助: サンプル / モック導線</summary>
                    <div className="classify-devtools-actions">
                        <button
                            type="button"
                            className="classify-secondary-button"
                            onClick={() => {
                                void handleGenerateSample();
                            }}
                            disabled={pending || fetchPending}
                        >
                            50 件サンプルを生成
                        </button>
                        <button
                            type="button"
                            className="classify-secondary-button"
                            onClick={() => {
                                void handleShowMockResults();
                            }}
                            disabled={pending || fetchPending || messages.length === 0}
                        >
                            モック分類を表示
                        </button>
                    </div>
                </details>
            </section>

            <section className="classify-fetched" aria-label="取得メール一覧">
                <div className="classify-results-header">
                    <div>
                        <h2>取得メール一覧</h2>
                        <p>
                            取得済み {messages.length} 件。Claude 分類後は分類状態もここで確認できます。
                        </p>
                    </div>
                </div>
                <div className="classify-table-wrap">
                    <table>
                        <thead>
                            <tr>
                                <th scope="col">未読</th>
                                <th scope="col">差出人 / 件名</th>
                                <th scope="col">スニペット</th>
                                <th scope="col">分類状態</th>
                                <th scope="col">メッセージID</th>
                                <th scope="col">詳細</th>
                            </tr>
                        </thead>
                        <tbody>
                            {messages.map((message) => {
                                const result = resultByMessageID.get(message.id);
                                return (
                                    <tr key={message.id}>
                                        <td>
                                            <p className={`unread-chip ${message.unread ? 'unread' : 'read'}`}>
                                                {message.unread ? '未読' : '既読'}
                                            </p>
                                        </td>
                                        <td>
                                            <p className="result-primary">{message.from || '不明な差出人'}</p>
                                            <p className="result-secondary">{message.subject || '(件名なし)'}</p>
                                        </td>
                                        <td className="reason-text">{message.snippet || '-'}</td>
                                        <td>
                                            {result ? (
                                                <>
                                                    <p className={`category-chip category-${result.category}`}>
                                                        {categoryLabel(result.category)}
                                                    </p>
                                                    <p className={`review-chip review-${result.reviewLevel}`}>
                                                        {reviewLabel(result.reviewLevel)}
                                                    </p>
                                                </>
                                            ) : (
                                                <p className="result-secondary">未分類</p>
                                            )}
                                        </td>
                                        <td>
                                            <p className="result-secondary classify-id">{message.id}</p>
                                        </td>
                                        <td>
                                            <button
                                                type="button"
                                                className="classify-secondary-button"
                                                onClick={() => {
                                                    void handleOpenMessageDetail(message.id);
                                                }}
                                                disabled={messageDetailPending}
                                            >
                                                {messageDetailPending && selectedMessageID === message.id
                                                    ? '読込中...'
                                                    : '詳細を開く'}
                                            </button>
                                        </td>
                                    </tr>
                                );
                            })}
                        </tbody>
                    </table>
                    {messages.length === 0 ? (
                        <p className="classify-empty">実メールが未取得です。上部の取得条件を指定して読み込んでください。</p>
                    ) : null}
                </div>
                {selectedMessageID ? (
                    <div className="classify-message-detail">
                        <div className="classify-message-detail-header">
                            <h3>メール詳細</h3>
                            <button
                                type="button"
                                className="classify-secondary-button"
                                onClick={clearMessageDetail}
                            >
                                閉じる
                            </button>
                        </div>
                        {messageDetailPending ? (
                            <p className="classify-inline-note">メール詳細を取得しています...</p>
                        ) : messageDetailError ? (
                            <p className="classify-error-note">{messageDetailError}</p>
                        ) : selectedMessageDetail ? (
                            <>
                                <dl className="classify-message-meta">
                                    <div>
                                        <dt>件名</dt>
                                        <dd>{selectedMessageDetail.subject || '(件名なし)'}</dd>
                                    </div>
                                    <div>
                                        <dt>差出人</dt>
                                        <dd>{selectedMessageDetail.from || '不明'}</dd>
                                    </div>
                                    <div>
                                        <dt>宛先</dt>
                                        <dd>{selectedMessageDetail.to || '-'}</dd>
                                    </div>
                                    <div>
                                        <dt>受信日時</dt>
                                        <dd>{selectedMessageDetail.date || '-'}</dd>
                                    </div>
                                    <div>
                                        <dt>Thread ID</dt>
                                        <dd className="classify-id">{selectedMessageDetail.threadID || '-'}</dd>
                                    </div>
                                    <div>
                                        <dt>Message ID</dt>
                                        <dd className="classify-id">{selectedMessageDetail.id}</dd>
                                    </div>
                                </dl>
                                <p className="classify-inline-note">
                                    ラベル: {selectedMessageDetail.labelIDs.length > 0 ? selectedMessageDetail.labelIDs.join(', ') : '-'}
                                </p>
                                <p className="classify-inline-note">
                                    スニペット: {selectedMessageDetail.snippet || '-'}
                                </p>
                                <details open>
                                    <summary>本文テキスト</summary>
                                    <pre className="classify-message-body">
                                        {selectedMessageDetail.bodyText || '(本文テキストは取得できませんでした)'}
                                    </pre>
                                </details>
                                {selectedMessageDetail.bodyHTML ? (
                                    <details>
                                        <summary>本文 HTML ソース</summary>
                                        <pre className="classify-message-body">{selectedMessageDetail.bodyHTML}</pre>
                                    </details>
                                ) : null}
                                {selectedMessageDetail.headers.length > 0 ? (
                                    <details>
                                        <summary>ヘッダ一覧</summary>
                                        <ul className="classify-header-list">
                                            {selectedMessageDetail.headers.map((header, index) => (
                                                <li key={`${header.name}-${index}`}>
                                                    <span className="header-name">{header.name}:</span>{' '}
                                                    <span className="header-value">{header.value}</span>
                                                </li>
                                            ))}
                                        </ul>
                                    </details>
                                ) : null}
                            </>
                        ) : null}
                    </div>
                ) : null}
            </section>

            <section className="classify-summary" aria-label="信頼度分岐サマリー">
                <article className="summary-card">
                    <h2>自動実行</h2>
                    <p>{reviewCounts.auto_apply} 件</p>
                </article>
                <article className="summary-card">
                    <h2>承認待ち</h2>
                    <p>{reviewCounts.review} 件</p>
                </article>
                <article className="summary-card">
                    <h2>要理由確認</h2>
                    <p>{reviewCounts.review_with_reason} 件</p>
                </article>
                <article className="summary-card">
                    <h2>保留</h2>
                    <p>{reviewCounts.hold} 件</p>
                </article>
            </section>

            <section className="classify-actions" aria-label="承認済み Gmail アクション実行">
                <div className="classify-actions-header">
                    <div>
                        <h2>承認済みアクション実行</h2>
                        <p>
                            選択中 {selectedDecisions.length} 件を Gmail 側へ反映します。
                            実行前に画面内の確認ステップを表示します。
                        </p>
                    </div>
                    <div className="classify-actions-buttons">
                        <button
                            type="button"
                            className="classify-secondary-button"
                            onClick={handleSelectAllExecutableRows}
                            disabled={rows.length === 0 || pending || actionPending || !isLiveDataMode}
                        >
                            実行候補を全選択
                        </button>
                        <button
                            type="button"
                            className="classify-secondary-button"
                            onClick={resetApprovalSelection}
                            disabled={selectedDecisions.length === 0 || pending || actionPending || !isLiveDataMode}
                        >
                            選択解除
                        </button>
                        <button
                            type="button"
                            className="classify-primary-button"
                            onClick={() => {
                                void handleExecuteSelectedActions();
                            }}
                            disabled={
                                actionPending ||
                                actionConfirmOpen ||
                                pending ||
                                fetchPending ||
                                !status.gmailConnected ||
                                !isLiveDataMode ||
                                selectedDecisions.length === 0
                            }
                        >
                            {actionPending ? 'Gmail 反映中...' : '選択した承認を Gmail に適用'}
                        </button>
                    </div>
                </div>
                {!status.gmailConnected ? (
                    <p className="classify-inline-note">
                        Gmail 接続未確認のため実行できません。Settings で接続確認を先に行ってください。
                    </p>
                ) : null}
                {actionConfirmOpen && isLiveDataMode ? (
                    <div className="classify-confirm-panel">
                        <p>
                            選択した {selectedDecisions.length} 件を Gmail に反映します。
                            {selectedDeleteCount > 0
                                ? ` このうち ${selectedDeleteCount} 件は削除されます。`
                                : ' 削除対象は含まれていません。'}
                        </p>
                        <div className="classify-actions-buttons">
                            <button
                                type="button"
                                className="classify-secondary-button"
                                onClick={() => {
                                    setActionConfirmOpen(false);
                                }}
                                disabled={actionPending}
                            >
                                キャンセル
                            </button>
                            <button
                                type="button"
                                className="classify-primary-button"
                                onClick={() => {
                                    void handleConfirmExecuteSelectedActions();
                                }}
                                disabled={actionPending}
                            >
                                実行する
                            </button>
                        </div>
                    </div>
                ) : null}
                {lastActionResult ? (
                    <div className="classify-action-result">
                        <p className={lastActionResult.success ? 'classify-inline-note' : 'classify-error-note'}>
                            {lastActionResult.message}
                        </p>
                        <dl className="classify-action-stats">
                            <div>
                                <dt>処理対象</dt>
                                <dd>{lastActionResult.processedCount} 件</dd>
                            </div>
                            <div>
                                <dt>成功</dt>
                                <dd>{lastActionResult.successCount} 件</dd>
                            </div>
                            <div>
                                <dt>失敗</dt>
                                <dd>{lastActionResult.failureCount} 件</dd>
                            </div>
                            <div>
                                <dt>スキップ</dt>
                                <dd>{lastActionResult.skippedCount} 件</dd>
                            </div>
                            <div>
                                <dt>削除</dt>
                                <dd>{lastActionResult.deletedCount} 件</dd>
                            </div>
                            <div>
                                <dt>アーカイブ</dt>
                                <dd>{lastActionResult.archivedCount} 件</dd>
                            </div>
                            <div>
                                <dt>既読化</dt>
                                <dd>{lastActionResult.markedReadCount} 件</dd>
                            </div>
                            <div>
                                <dt>ラベル付与</dt>
                                <dd>{lastActionResult.labeledCount} 件</dd>
                            </div>
                        </dl>
                        {lastActionResult.createdLabels.length > 0 ? (
                            <p className="classify-inline-note">
                                新規作成ラベル: {lastActionResult.createdLabels.join(', ')}
                            </p>
                        ) : null}
                        {lastActionResult.tokenRefreshed ? (
                            <p className="classify-inline-note">
                                実行前に Google トークンを更新しました。
                            </p>
                        ) : null}
                        {lastActionResult.failures.length > 0 ? (
                            <ul className="classify-failure-list">
                                {lastActionResult.failures.map((failure) => (
                                    <li key={`${failure.messageID}-${failure.action}`}>
                                        {failure.messageID} / {actionKindLabel(failure.action)}: {failure.error}
                                    </li>
                                ))}
                            </ul>
                        ) : null}
                    </div>
                ) : null}
                {actionError ? <p className="classify-error-note">{actionError}</p> : null}
                {actionHistory.length > 0 ? (
                    <div className="classify-history">
                        <h3>実行履歴 (最新 5 件)</h3>
                        <ul>
                            {actionHistory.slice(0, 5).map((item, index) => (
                                <li key={`${item.at}-${index}`}>
                                    {formatDateTime(item.at)} / 選択 {item.selectedCount} 件 / 成功 {item.result.successCount} /
                                    失敗 {item.result.failureCount} / スキップ {item.result.skippedCount}
                                </li>
                            ))}
                        </ul>
                    </div>
                ) : null}
            </section>

            <section className="classify-results" aria-label="分類結果一覧">
                <div className="classify-results-header">
                    <div>
                        <h2>分類結果一覧</h2>
                        <p>
                            実行候補 {approvalCandidateCount} 件中 {selectedApprovalCount} 件を選択中
                        </p>
                    </div>
                    <div className="classify-filter-group" role="group" aria-label="レビュー状態で絞り込み">
                        {reviewFilterOptions.map((option) => (
                            <button
                                key={option.value}
                                type="button"
                                className={`filter-button ${filter === option.value ? 'active' : ''}`}
                                aria-pressed={filter === option.value}
                                onClick={() => {
                                    setFilter(option.value);
                                }}
                            >
                                {option.label}
                            </button>
                        ))}
                    </div>
                </div>
                <p className="classify-inline-note">
                    分類修正を保存すると、同一送信者/ドメインで 3 回以上の修正時に Blocklist 画面へ提案されます。
                </p>
                {correctionMessage ? <p className="classify-inline-note">{correctionMessage}</p> : null}
                {correctionError ? <p className="classify-error-note">{correctionError}</p> : null}

                <div className="classify-table-wrap">
                    <table>
                        <thead>
                            <tr>
                                <th scope="col">承認</th>
                                <th scope="col">差出人 / 件名</th>
                                <th scope="col">分類元</th>
                                <th scope="col">分類</th>
                                <th scope="col">修正</th>
                                <th scope="col">信頼度</th>
                                <th scope="col">推奨アクション</th>
                                <th scope="col">理由</th>
                            </tr>
                        </thead>
                        <tbody>
                            {filteredRows.map((row) => {
                                const numericConfidence = Number.isFinite(row.result.confidence)
                                    ? row.result.confidence
                                    : 0;
                                const confidencePercent = Math.min(
                                    100,
                                    Math.max(0, Math.round(numericConfidence * 100)),
                                );
                                const approvalEnabled = row.result.reviewLevel !== 'hold' && isLiveDataMode;
                                const resolvedCategory =
                                    correctedCategories[row.result.messageID] ?? row.result.category;
                                return (
                                    <tr key={row.result.messageID}>
                                        <td>
                                            <input
                                                type="checkbox"
                                                checked={Boolean(selectedForApproval[row.result.messageID])}
                                                onChange={() => {
                                                    handleToggleApproval(row.result.messageID);
                                                }}
                                                disabled={!approvalEnabled}
                                                aria-label={`${row.result.messageID} を承認候補にする`}
                                            />
                                        </td>
                                        <td>
                                            <p className="result-primary">
                                                {row.message?.from ?? '不明な差出人'}
                                            </p>
                                            <p className="result-secondary">
                                                {row.message?.subject ?? row.result.messageID}
                                            </p>
                                            {row.message?.snippet ? (
                                                <p className="result-secondary">{row.message.snippet}</p>
                                            ) : null}
                                        </td>
                                        <td>
                                            <p className={`source-chip source-${row.result.source}`}>
                                                {row.result.source === 'blocklist' ? 'Blocklist' : 'Claude'}
                                            </p>
                                        </td>
                                        <td>
                                            <p className={`category-chip category-${resolvedCategory}`}>
                                                {categoryLabel(resolvedCategory)}
                                            </p>
                                            <p className={`review-chip review-${row.result.reviewLevel}`}>
                                                {reviewLabel(row.result.reviewLevel)}
                                            </p>
                                        </td>
                                        <td>
                                            <select
                                                className="category-edit-select"
                                                value={resolvedCategory}
                                                onChange={(event) => {
                                                    void handleCategoryCorrection(
                                                        row,
                                                        event.target.value as ClassificationCategory,
                                                    );
                                                }}
                                                disabled={
                                                    pending ||
                                                    !isLiveDataMode ||
                                                    Boolean(correctionPendingByID[row.result.messageID])
                                                }
                                                aria-label={`${row.result.messageID} の分類修正`}
                                            >
                                                {categoryOptions.map((option) => (
                                                    <option key={option.value} value={option.value}>
                                                        {option.label}
                                                    </option>
                                                ))}
                                            </select>
                                        </td>
                                        <td>
                                            <p className="confidence-value">{confidencePercent}%</p>
                                            <div className="confidence-track" aria-hidden>
                                                <div
                                                    className="confidence-fill"
                                                    style={{ width: `${confidencePercent}%` }}
                                                />
                                            </div>
                                        </td>
                                        <td className="action-text">
                                            {recommendedAction(resolvedCategory, row.result.reviewLevel)}
                                        </td>
                                        <td className="reason-text">{row.result.reason}</td>
                                    </tr>
                                );
                            })}
                        </tbody>
                    </table>
                    {messages.length === 0 ? (
                        <p className="classify-empty">実メールが未取得です。上部の取得条件を指定して読み込んでください。</p>
                    ) : filteredRows.length === 0 && results.length === 0 ? (
                        <p className="classify-empty">分類結果がありません。Claude 分類を実行してください。</p>
                    ) : filteredRows.length === 0 ? (
                        <p className="classify-empty">該当する分類結果はありません。</p>
                    ) : null}
                </div>
            </section>
        </div>
    );
}

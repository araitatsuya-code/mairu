import './ClassifyPage.css';

import { useMemo, useState } from 'react';

import {
    classifyEmails,
    type ClassificationCategory,
    type ClassificationResponse,
    type ClassificationReviewLevel,
    type EmailSummary,
    executeGmailActions,
    recordClassificationRun,
    recordClassificationCorrection,
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

const sampleMessageCount = 50;

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

function formatClassifiedAt(value: string | null): string {
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

export function ClassifyPage({ status }: ClassifyPageProps) {
    const [messages, setMessages] = useState<EmailSummary[]>(() =>
        buildSampleMessages(sampleMessageCount));
    const [results, setResults] = useState<ClassificationResponse['results']>(() =>
        buildMockResults(buildSampleMessages(sampleMessageCount)));
    const [filter, setFilter] = useState<ReviewFilter>('all');
    const [selectedForApproval, setSelectedForApproval] = useState<Record<string, boolean>>({});
    const [correctedCategories, setCorrectedCategories] = useState<
        Record<string, ClassificationCategory>
    >({});
    const [classifyModel, setClassifyModel] = useState('');
    const [pending, setPending] = useState(false);
    const [error, setError] = useState<string | null>(null);
    const [actionPending, setActionPending] = useState(false);
    const [actionError, setActionError] = useState<string | null>(null);
    const [correctionMessage, setCorrectionMessage] = useState<string | null>(null);
    const [correctionError, setCorrectionError] = useState<string | null>(null);
    const [correctionPendingByID, setCorrectionPendingByID] = useState<Record<string, boolean>>({});
    const [lastActionResult, setLastActionResult] = useState<ExecuteGmailActionsResult | null>(null);
    const [loggingMessage, setLoggingMessage] = useState<string | null>(null);
    const [lastClassifiedAt, setLastClassifiedAt] = useState<string | null>(
        new Date().toISOString(),
    );

    const messageByID = useMemo(() => {
        return new Map(messages.map((message) => [message.id, message]));
    }, [messages]);

    const rows = useMemo<ClassifiedRow[]>(() => {
        return results.map((result) => ({
            result,
            message: messageByID.get(result.messageID),
        }));
    }, [messageByID, results]);

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
        return rows.filter((row) => row.result.reviewLevel !== 'auto_apply').length;
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

    function resetApprovalSelection() {
        setSelectedForApproval({});
    }

    function resetCorrections() {
        setCorrectedCategories({});
        setCorrectionMessage(null);
        setCorrectionError(null);
        setCorrectionPendingByID({});
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

    async function handleGenerateSample() {
        const generated = buildSampleMessages(sampleMessageCount);
        const nextResults = buildMockResults(generated);
        setMessages(generated);
        setResults(nextResults);
        setError(null);
        setActionError(null);
        setLastActionResult(null);
        setLastClassifiedAt(new Date().toISOString());
        resetApprovalSelection();
        resetCorrections();
        try {
            await persistClassificationLogs(generated, nextResults);
        } catch (cause) {
            setError(cause instanceof Error ? cause.message : '分類ログ保存に失敗しました。');
        }
    }

    async function handleShowMockResults() {
        const nextResults = buildMockResults(messages);
        setResults(nextResults);
        setError(null);
        setActionError(null);
        setLastActionResult(null);
        setLastClassifiedAt(new Date().toISOString());
        resetApprovalSelection();
        resetCorrections();
        try {
            await persistClassificationLogs(messages, nextResults);
        } catch (cause) {
            setError(cause instanceof Error ? cause.message : '分類ログ保存に失敗しました。');
        }
    }

    async function handleClassifyWithClaude() {
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

            setResults(response.results);
            setLastClassifiedAt(new Date().toISOString());
            setActionError(null);
            setLastActionResult(null);
            resetApprovalSelection();
            resetCorrections();
            await persistClassificationLogs(messages, response.results);
        } catch (cause) {
            const message =
                cause instanceof Error
                    ? cause.message
                    : 'Claude 分類に失敗しました。';
            setError(message);
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

    async function handleCategoryCorrection(row: ClassifiedRow, nextCategory: ClassificationCategory) {
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
            setCorrectionError(cause instanceof Error ? cause.message : '修正履歴の保存に失敗しました。');
        } finally {
            setCorrectionPendingByID((previous) => ({
                ...previous,
                [messageID]: false,
            }));
        }
    }

    async function handleExecuteSelectedActions() {
        if (selectedDecisions.length === 0) {
            setActionError('実行対象を選択してください。');
            return;
        }

        const deleteCount = selectedDecisions.filter((item) => item.category === 'junk').length;
        const confirmed = window.confirm(
            [
                `選択した ${selectedDecisions.length} 件を Gmail に反映します。`,
                deleteCount > 0 ? `このうち ${deleteCount} 件は削除されます。` : '削除対象は含まれていません。',
                '実行しますか？',
            ].join('\n'),
        );
        if (!confirmed) {
            return;
        }

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
            resetApprovalSelection();

            if (!result.success) {
                setActionError(result.message);
            }
        } catch (cause) {
            const message =
                cause instanceof Error
                    ? cause.message
                    : 'Gmail アクション実行に失敗しました。';
            setActionError(message);
        } finally {
            setActionPending(false);
        }
    }

    return (
        <div className="classify-page">
            <section className="classify-hero">
                <p className="classify-eyebrow">MAIRU-009 / #9</p>
                <h1>分類確認</h1>
                <p className="classify-lead">
                    50 件の分類結果を信頼度で振り分け、承認対象を判断するための画面です。
                    自動実行候補と要確認候補を分けて確認できます。
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

            <section className="classify-controls" aria-label="分類実行コントロール">
                <div className="classify-control-row">
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
                        className="classify-secondary-button"
                        onClick={() => {
                            void handleGenerateSample();
                        }}
                        disabled={pending}
                    >
                        50 件サンプルを再生成
                    </button>
                    <button
                        type="button"
                        className="classify-secondary-button"
                        onClick={() => {
                            void handleShowMockResults();
                        }}
                        disabled={pending}
                    >
                        モック分類を表示
                    </button>
                    <button
                        type="button"
                        className="classify-primary-button"
                        onClick={() => {
                            void handleClassifyWithClaude();
                        }}
                        disabled={pending || !status.claudeConfigured}
                    >
                        {pending ? 'Claude 分類中...' : 'Claude で分類'}
                    </button>
                </div>
                <p className="classify-inline-note">
                    対象件数: {messages.length} 件 / 最終更新: {formatClassifiedAt(lastClassifiedAt)}
                </p>
                {!status.claudeConfigured ? (
                    <p className="classify-inline-note">
                        Claude API キー未設定のため、実 API 実行は無効化しています。Settings から設定できます。
                    </p>
                ) : null}
                {error ? <p className="classify-error-note">{error}</p> : null}
                {loggingMessage ? <p className="classify-inline-note">{loggingMessage}</p> : null}
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
                            削除を含む場合は確認ダイアログを表示します。
                        </p>
                    </div>
                    <button
                        type="button"
                        className="classify-primary-button"
                        onClick={() => {
                            void handleExecuteSelectedActions();
                        }}
                        disabled={
                            actionPending ||
                            pending ||
                            !status.gmailConnected ||
                            selectedDecisions.length === 0
                        }
                    >
                        {actionPending ? 'Gmail 反映中...' : '選択した承認を Gmail に適用'}
                    </button>
                </div>
                {!status.gmailConnected ? (
                    <p className="classify-inline-note">
                        Gmail 接続未確認のため実行できません。Settings で接続確認を先に行ってください。
                    </p>
                ) : null}
                {lastActionResult ? (
                    <div className="classify-action-result">
                        <p className={lastActionResult.success ? 'classify-inline-note' : 'classify-error-note'}>
                            {lastActionResult.message}
                        </p>
                        <dl className="classify-action-stats">
                            <div>
                                <dt>成功</dt>
                                <dd>{lastActionResult.successCount} 件</dd>
                            </div>
                            <div>
                                <dt>失敗</dt>
                                <dd>{lastActionResult.failureCount} 件</dd>
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
            </section>

            <section className="classify-results" aria-label="分類結果一覧">
                <div className="classify-results-header">
                    <div>
                        <h2>分類結果一覧</h2>
                        <p>
                            承認候補 {approvalCandidateCount} 件中 {selectedApprovalCount} 件を選択中
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
                                const approvalEnabled = row.result.reviewLevel !== 'auto_apply';
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
                                                disabled={pending || Boolean(correctionPendingByID[row.result.messageID])}
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
                    {filteredRows.length === 0 ? (
                        <p className="classify-empty">該当する分類結果はありません。</p>
                    ) : null}
                </div>
            </section>
        </div>
    );
}

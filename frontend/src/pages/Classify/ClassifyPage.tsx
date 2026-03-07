import './ClassifyPage.css';

import { useMemo, useState } from 'react';

import {
    classifyEmails,
    type ClassificationCategory,
    type ClassificationResponse,
    type ClassificationReviewLevel,
    type EmailSummary,
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

export function ClassifyPage({ status }: ClassifyPageProps) {
    const [messages, setMessages] = useState<EmailSummary[]>(() =>
        buildSampleMessages(sampleMessageCount));
    const [results, setResults] = useState<ClassificationResponse['results']>(() =>
        buildMockResults(buildSampleMessages(sampleMessageCount)));
    const [filter, setFilter] = useState<ReviewFilter>('all');
    const [selectedForApproval, setSelectedForApproval] = useState<Record<string, boolean>>({});
    const [classifyModel, setClassifyModel] = useState('');
    const [pending, setPending] = useState(false);
    const [error, setError] = useState<string | null>(null);
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

    function resetApprovalSelection() {
        setSelectedForApproval({});
    }

    function handleGenerateSample() {
        const generated = buildSampleMessages(sampleMessageCount);
        setMessages(generated);
        setResults(buildMockResults(generated));
        setError(null);
        setLastClassifiedAt(new Date().toISOString());
        resetApprovalSelection();
    }

    function handleShowMockResults() {
        setResults(buildMockResults(messages));
        setError(null);
        setLastClassifiedAt(new Date().toISOString());
        resetApprovalSelection();
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
            resetApprovalSelection();
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

    return (
        <div className="classify-page">
            <section className="classify-hero">
                <p className="classify-eyebrow">MAIRU-008 / #8</p>
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
                        onClick={handleGenerateSample}
                        disabled={pending}
                    >
                        50 件サンプルを再生成
                    </button>
                    <button
                        type="button"
                        className="classify-secondary-button"
                        onClick={handleShowMockResults}
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

                <div className="classify-table-wrap">
                    <table>
                        <thead>
                            <tr>
                                <th scope="col">承認</th>
                                <th scope="col">差出人 / 件名</th>
                                <th scope="col">分類</th>
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
                                            <p className={`category-chip category-${row.result.category}`}>
                                                {categoryLabel(row.result.category)}
                                            </p>
                                            <p className={`review-chip review-${row.result.reviewLevel}`}>
                                                {reviewLabel(row.result.reviewLevel)}
                                            </p>
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
                                            {recommendedAction(row.result.category, row.result.reviewLevel)}
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

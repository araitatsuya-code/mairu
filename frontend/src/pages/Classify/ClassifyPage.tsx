import './ClassifyPage.css';

type ReviewBucket = 'auto' | 'approval' | 'check';
type ClassificationCategory = 'important' | 'newsletter' | 'junk' | 'archive' | 'unread_priority';
type ClassificationReviewLevel = 'auto_apply' | 'review' | 'review_with_reason' | 'hold';

type ClassificationRow = {
    messageID: string;
    from: string;
    subject: string;
    confidence: number;
    category: ClassificationCategory;
    reviewLevel: ClassificationReviewLevel;
    reason: string;
};

const categoryLabelMap: Record<ClassificationCategory, string> = {
    important: '重要',
    newsletter: 'ニュースレター',
    junk: '不要',
    archive: '保管',
    unread_priority: '未読優先',
};

const categoryActionMap: Record<ClassificationCategory, string> = {
    important: '受信トレイに残してスターを付与',
    newsletter: 'ニュースレターラベルを付けて既読化',
    junk: '迷惑候補としてアーカイブ',
    archive: '通常アーカイブへ移動',
    unread_priority: '未読優先ラベルを付けて上位に固定',
};

const mockRows: ClassificationRow[] = Array.from({ length: 50 }, (_, index) => {
    const position = index + 1;
    const categoryCycle: ClassificationCategory[] = ['important', 'newsletter', 'junk', 'archive', 'unread_priority'];
    const reviewCycle: ClassificationReviewLevel[] = ['auto_apply', 'review', 'review_with_reason', 'review', 'hold'];
    const confidenceBase = [0.96, 0.88, 0.72, 0.81, 0.67][index % 5];

    return {
        messageID: `msg-${String(position).padStart(3, '0')}`,
        from: `sender${position}@example.com`,
        subject: `分類候補メール ${position}`,
        confidence: Math.max(0.51, confidenceBase - Math.floor(index / 10) * 0.02),
        category: categoryCycle[index % categoryCycle.length],
        reviewLevel: reviewCycle[index % reviewCycle.length],
        reason: `本文要約と送信元傾向から ${categoryLabelMap[categoryCycle[index % categoryCycle.length]]} に分類しました。`,
    };
});

function resolveReviewBucket(row: ClassificationRow): ReviewBucket {
    if (row.reviewLevel === 'auto_apply' && row.confidence >= 0.9) {
        return 'auto';
    }
    if (row.reviewLevel === 'hold' || row.reviewLevel === 'review_with_reason' || row.confidence < 0.75) {
        return 'check';
    }
    return 'approval';
}

function formatConfidence(confidence: number): string {
    return `${Math.round(confidence * 100)}%`;
}

function bucketLabel(bucket: ReviewBucket): string {
    if (bucket === 'auto') {
        return '自動実行';
    }
    if (bucket === 'approval') {
        return '承認待ち';
    }
    return '要確認';
}

function bucketAction(row: ClassificationRow, bucket: ReviewBucket): string {
    if (bucket === 'check') {
        return `要確認: ${categoryActionMap[row.category]}`;
    }
    if (bucket === 'approval') {
        return `承認後に実行: ${categoryActionMap[row.category]}`;
    }
    return `自動実行予定: ${categoryActionMap[row.category]}`;
}

function bucketClassName(bucket: ReviewBucket): string {
    if (bucket === 'auto') {
        return 'auto';
    }
    if (bucket === 'approval') {
        return 'approval';
    }
    return 'check';
}

export function ClassifyPage() {
    const summarized = mockRows.reduce(
        (accumulator, row) => {
            const bucket = resolveReviewBucket(row);
            accumulator[bucket] += 1;
            return accumulator;
        },
        { auto: 0, approval: 0, check: 0 },
    );

    return (
        <div className="classify-page">
            <section className="classify-hero">
                <p className="classify-eyebrow">MAIRU-008 / #8</p>
                <h1>分類確認</h1>
                <p className="classify-lead">
                    50 件の分類結果を一覧で確認し、信頼度と推奨アクションを見ながら「自動実行 / 承認待ち / 要確認」を判断できます。
                </p>
            </section>

            <section className="classify-summary" aria-label="分類結果サマリー">
                <article className="classify-summary-card auto">
                    <h2>自動実行</h2>
                    <p>{summarized.auto} 件</p>
                </article>
                <article className="classify-summary-card approval">
                    <h2>承認待ち</h2>
                    <p>{summarized.approval} 件</p>
                </article>
                <article className="classify-summary-card check">
                    <h2>要確認</h2>
                    <p>{summarized.check} 件</p>
                </article>
            </section>

            <section className="classify-table-wrap" aria-label="分類結果一覧">
                <table className="classify-table">
                    <thead>
                        <tr>
                            <th scope="col">件名 / 送信元</th>
                            <th scope="col">分類</th>
                            <th scope="col">信頼度</th>
                            <th scope="col">状態</th>
                            <th scope="col">推奨アクション</th>
                        </tr>
                    </thead>
                    <tbody>
                        {mockRows.map((row) => {
                            const bucket = resolveReviewBucket(row);
                            return (
                                <tr key={row.messageID}>
                                    <td>
                                        <p className="classify-subject">{row.subject}</p>
                                        <p className="classify-meta">{row.from}</p>
                                    </td>
                                    <td>{categoryLabelMap[row.category]}</td>
                                    <td>
                                        <div className="classify-confidence">
                                            <span>{formatConfidence(row.confidence)}</span>
                                            <div className="classify-confidence-bar" role="presentation">
                                                <div
                                                    className={`classify-confidence-fill ${bucketClassName(bucket)}`}
                                                    style={{ width: `${Math.round(row.confidence * 100)}%` }}
                                                />
                                            </div>
                                        </div>
                                    </td>
                                    <td>
                                        <span className={`classify-badge ${bucketClassName(bucket)}`}>{bucketLabel(bucket)}</span>
                                    </td>
                                    <td>
                                        <p className="classify-action">{bucketAction(row, bucket)}</p>
                                        <p className="classify-meta">{row.reason}</p>
                                    </td>
                                </tr>
                            );
                        })}
                    </tbody>
                </table>
            </section>
        </div>
    );
}

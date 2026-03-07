import './ExportPage.css';

import { useState } from 'react';

import {
    exportBlocklistJSON,
    exportDailyLogsCSV,
    exportDailyLogsJSON,
    exportImportantSummaryCSV,
    exportImportantSummaryPDF,
    exportProcessedMailCSV,
    exportProcessedMailJSON,
    importBlocklistJSON,
    type OperationResult,
    type RuntimeStatus,
} from '../../lib/runtime';

type ExportPageProps = {
    status: RuntimeStatus;
};

function isCancellationMessage(message: string): boolean {
    return message.includes('キャンセル');
}

export function ExportPage({ status }: ExportPageProps) {
    const [busyKey, setBusyKey] = useState<string | null>(null);
    const [message, setMessage] = useState<string | null>(null);
    const [error, setError] = useState<string | null>(null);

    async function runAction(key: string, action: () => Promise<OperationResult>) {
        setBusyKey(key);
        setMessage(null);
        setError(null);

        try {
            const result = await action();
            if (!result.success) {
                if (isCancellationMessage(result.message)) {
                    setMessage(result.message);
                } else {
                    setError(result.message);
                }
                return;
            }

            setMessage(result.message);
        } catch (cause) {
            setError(cause instanceof Error ? cause.message : 'エクスポート操作に失敗しました。');
        } finally {
            setBusyKey(null);
        }
    }

    return (
        <div className="export-page">
            <section className="export-hero">
                <p className="export-eyebrow">MAIRU-012 / #12</p>
                <h1>エクスポート</h1>
                <p className="export-lead">
                    処理済みメール、ブロックリスト、重要メールサマリー、日別分類ログを
                    CSV / JSON / PDF で持ち出せます。
                </p>
                <div className="export-chips">
                    <span className={`export-chip ${status.databaseReady ? 'ready' : 'pending'}`}>
                        SQLite: {status.databaseReady ? '利用可能' : '未初期化'}
                    </span>
                    <span className={`export-chip ${status.gmailConnected ? 'ready' : 'pending'}`}>
                        Gmail 接続: {status.gmailConnected ? '確認済み' : '未確認'}
                    </span>
                </div>
            </section>

            <section className="export-grid">
                <article className="export-card">
                    <h2>処理済みメール一覧</h2>
                    <p>Gmail に反映したメールの履歴を、監査や振り返り向けに保存します。</p>
                    <div className="export-actions">
                        <button
                            type="button"
                            onClick={() => {
                                void runAction('processed-csv', exportProcessedMailCSV);
                            }}
                            disabled={!status.databaseReady || busyKey !== null}
                        >
                            {busyKey === 'processed-csv' ? 'CSV 出力中...' : 'CSV を保存'}
                        </button>
                        <button
                            type="button"
                            className="secondary"
                            onClick={() => {
                                void runAction('processed-json', exportProcessedMailJSON);
                            }}
                            disabled={!status.databaseReady || busyKey !== null}
                        >
                            {busyKey === 'processed-json' ? 'JSON 出力中...' : 'JSON を保存'}
                        </button>
                    </div>
                </article>

                <article className="export-card">
                    <h2>ブロックリスト</h2>
                    <p>blocklist を JSON でバックアップし、別環境や再セットアップ時に取り込めます。</p>
                    <div className="export-actions">
                        <button
                            type="button"
                            onClick={() => {
                                void runAction('blocklist-export', exportBlocklistJSON);
                            }}
                            disabled={!status.databaseReady || busyKey !== null}
                        >
                            {busyKey === 'blocklist-export' ? 'JSON 出力中...' : 'JSON を保存'}
                        </button>
                        <button
                            type="button"
                            className="secondary"
                            onClick={() => {
                                void runAction('blocklist-import', importBlocklistJSON);
                            }}
                            disabled={!status.databaseReady || busyKey !== null}
                        >
                            {busyKey === 'blocklist-import' ? '取込中...' : 'JSON を取り込む'}
                        </button>
                    </div>
                </article>

                <article className="export-card">
                    <h2>重要メールサマリー</h2>
                    <p>重要判定されたメールだけを抽出し、CSV または PDF で共有しやすくします。</p>
                    <div className="export-actions">
                        <button
                            type="button"
                            onClick={() => {
                                void runAction('important-csv', exportImportantSummaryCSV);
                            }}
                            disabled={!status.databaseReady || busyKey !== null}
                        >
                            {busyKey === 'important-csv' ? 'CSV 出力中...' : 'CSV を保存'}
                        </button>
                        <button
                            type="button"
                            className="secondary"
                            onClick={() => {
                                void runAction('important-pdf', exportImportantSummaryPDF);
                            }}
                            disabled={!status.databaseReady || busyKey !== null}
                        >
                            {busyKey === 'important-pdf' ? 'PDF 出力中...' : 'PDF を保存'}
                        </button>
                    </div>
                </article>

                <article className="export-card">
                    <h2>日別分類ログ</h2>
                    <p>カテゴリ別件数とレビュー分岐を日単位で集計し、分類傾向を確認できます。</p>
                    <div className="export-actions">
                        <button
                            type="button"
                            onClick={() => {
                                void runAction('daily-csv', exportDailyLogsCSV);
                            }}
                            disabled={!status.databaseReady || busyKey !== null}
                        >
                            {busyKey === 'daily-csv' ? 'CSV 出力中...' : 'CSV を保存'}
                        </button>
                        <button
                            type="button"
                            className="secondary"
                            onClick={() => {
                                void runAction('daily-json', exportDailyLogsJSON);
                            }}
                            disabled={!status.databaseReady || busyKey !== null}
                        >
                            {busyKey === 'daily-json' ? 'JSON 出力中...' : 'JSON を保存'}
                        </button>
                    </div>
                </article>
            </section>

            <section className="export-notes">
                <h2>補足</h2>
                <ul>
                    <li>分類ログは Classify 画面で「モック分類を表示」または「Claude で分類」を実行すると蓄積されます。</li>
                    <li>処理済みメール一覧は Gmail 反映を実行した履歴から生成されます。</li>
                    <li>`mbox` は次フェーズ扱いのため、この画面には含めていません。</li>
                </ul>
                {message ? <p className="export-note success">{message}</p> : null}
                {error ? <p className="export-note error">{error}</p> : null}
            </section>
        </div>
    );
}

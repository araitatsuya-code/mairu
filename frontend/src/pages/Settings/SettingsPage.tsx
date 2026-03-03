import './SettingsPage.css';

import { useState } from 'react';

import {
    cancelGoogleLogin,
    startGoogleLogin,
    type GoogleLoginResult,
    type RuntimeStatus,
} from '../../lib/runtime';

type SettingsPageProps = {
    appName: string;
    status: RuntimeStatus;
    onStatusRefresh: () => Promise<RuntimeStatus>;
};

type StatusCardProps = {
    label: string;
    readyLabel: string;
    pendingLabel: string;
    ready: boolean;
};

function StatusCard({ label, readyLabel, pendingLabel, ready }: StatusCardProps) {
    return (
        <article className="status-card">
            <span className="status-label">{label}</span>
            <p className={`status-value ${ready ? 'ready' : 'pending'}`}>
                {ready ? readyLabel : pendingLabel}
            </p>
        </article>
    );
}

function formatLastRun(lastRunAt: string | null): string {
    if (!lastRunAt) {
        return 'まだ実行されていません';
    }

    const parsed = new Date(lastRunAt);
    if (Number.isNaN(parsed.getTime())) {
        return lastRunAt;
    }

    return new Intl.DateTimeFormat('ja-JP', {
        dateStyle: 'medium',
        timeStyle: 'short',
    }).format(parsed);
}

export function SettingsPage({ appName, status, onStatusRefresh }: SettingsPageProps) {
    const [loginPending, setLoginPending] = useState(false);
    const [loginError, setLoginError] = useState<string | null>(null);
    const [lastLoginResult, setLastLoginResult] = useState<GoogleLoginResult | null>(null);
    const [loginNote, setLoginNote] = useState<string | null>(null);
    const googleStateLabel = status.authorized
        ? '認可コード取得済み'
        : status.googleConfigured
          ? 'ログイン可能'
          : 'Client ID待ち';

    async function handleGoogleLogin() {
        setLoginPending(true);
        setLoginError(null);
        setLastLoginResult(null);
        setLoginNote(null);

        try {
            const result = await startGoogleLogin();
            setLastLoginResult(result);
            await onStatusRefresh();
        } catch (cause) {
            const message =
                cause instanceof Error
                    ? cause.message
                    : 'Google ログインに失敗しました。';
            if (message.includes('中断しました') || message.includes('context canceled')) {
                setLoginNote('ログイン処理を中断しました。再試行できます。');
            } else {
                setLoginError(message);
            }
            await onStatusRefresh();
        } finally {
            setLoginPending(false);
        }
    }

    async function handleCancelLogin() {
        setLoginNote('中断しています...');
        setLoginError(null);
        const cancelled = await cancelGoogleLogin();
        if (!cancelled) {
            setLoginNote('中断対象のログインが見つかりませんでした。');
            return;
        }
        await onStatusRefresh();
    }

    return (
        <div className="settings-page">
            <section className="settings-hero">
                <p className="settings-eyebrow">MAIRU-003 / #3</p>
                <h1>{appName} 設定ハブ</h1>
                <p className="settings-lead">
                    起動直後に必要な初期状態をここで確認し、OAuth、Claude API キー、
                    通知や自動実行の導線を段階的に接続していきます。
                </p>
            </section>

            <section className="settings-status-grid" aria-label="初期状態サマリー">
                <StatusCard
                    label="Google 認証"
                    readyLabel="認可コード取得済み"
                    pendingLabel="未接続"
                    ready={status.authorized}
                />
                <StatusCard
                    label="Claude API"
                    readyLabel="設定済み"
                    pendingLabel="未設定"
                    ready={status.claudeConfigured}
                />
                <StatusCard
                    label="ローカル保存"
                    readyLabel="利用可能"
                    pendingLabel="未初期化"
                    ready={status.databaseReady}
                />
            </section>

            <div className="settings-layout">
                <section className="settings-panel">
                    <h2>初期化フロー</h2>
                    <p className="settings-panel-copy">
                        後続 issue で実装する設定項目を、画面上で先に確認できるようにしています。
                        ここから認証状態、API キー状態、通知設定の表示領域を育てていきます。
                    </p>
                    <ul className="settings-list">
                        <li className="settings-item">
                            <div className="settings-item-header">
                                <h3 className="settings-item-title">Google OAuth ログイン</h3>
                                <span className={`state-chip ${status.authorized ? 'ready' : 'pending'}`}>
                                    {googleStateLabel}
                                </span>
                            </div>
                            <p className="settings-item-body">
                                Google OAuth の PKCE フローを使い、ブラウザ起動から localhost
                                リダイレクト受信までをこの場で完結させます。
                            </p>
                            <div className="settings-action-stack">
                                <div className="settings-action-row">
                                    <button
                                        className="settings-action-button"
                                        type="button"
                                        onClick={() => {
                                            void handleGoogleLogin();
                                        }}
                                        disabled={loginPending || !status.googleConfigured}
                                    >
                                        {loginPending ? 'Google ログイン待機中...' : 'Google でログイン'}
                                    </button>
                                    {loginPending ? (
                                        <button
                                            className="settings-cancel-button"
                                            type="button"
                                            onClick={() => {
                                                void handleCancelLogin();
                                            }}
                                        >
                                            中断
                                        </button>
                                    ) : null}
                                </div>
                                <p className="settings-inline-note">{status.authStatus}</p>
                                {lastLoginResult ? (
                                    <dl className="settings-result-grid">
                                        <div>
                                            <dt>結果</dt>
                                            <dd>{lastLoginResult.message}</dd>
                                        </div>
                                        <div>
                                            <dt>認可コード</dt>
                                            <dd>{lastLoginResult.codePreview || '受信済み'}</dd>
                                        </div>
                                        <div>
                                            <dt>リダイレクト</dt>
                                            <dd>{lastLoginResult.redirectURL}</dd>
                                        </div>
                                        <div>
                                            <dt>スコープ</dt>
                                            <dd>{lastLoginResult.scopes.join(', ')}</dd>
                                        </div>
                                    </dl>
                                ) : null}
                                {loginNote ? <p className="settings-inline-note">{loginNote}</p> : null}
                                {loginError ? <p className="settings-error-note">{loginError}</p> : null}
                            </div>
                        </li>
                        <li className="settings-item">
                            <div className="settings-item-header">
                                <h3 className="settings-item-title">Claude API キー管理</h3>
                                <span className={`state-chip ${status.claudeConfigured ? 'ready' : 'pending'}`}>
                                    {status.claudeConfigured ? '利用可能' : '準備中'}
                                </span>
                            </div>
                            <p className="settings-item-body">
                                OS キーチェーン連携を前提に、保存状態の確認と再入力導線を配置します。
                            </p>
                        </li>
                        <li className="settings-item">
                            <div className="settings-item-header">
                                <h3 className="settings-item-title">通知と自動実行</h3>
                                <span className="state-chip muted">後続 issue</span>
                            </div>
                            <p className="settings-item-body">
                                定期実行スケジュールや OS 通知の UI は、この領域に拡張します。
                            </p>
                        </li>
                    </ul>
                </section>

                <aside className="settings-aside">
                    <h2>起動時チェック</h2>
                    <p className="settings-aside-copy">
                        初期レンダリング時に Go 側の状態を読み取り、未設定箇所が分かる状態を作っています。
                    </p>
                    <ol className="settings-checklist">
                        <li>Wails 経由でアプリ名を取得</li>
                        <li>初期ステータスをロード</li>
                        <li>不足している設定領域を画面に表示</li>
                    </ol>
                    <div className="settings-meta">
                        <p className="settings-meta-label">前回の実行</p>
                        <p className="settings-meta-value">{formatLastRun(status.lastRunAt)}</p>
                    </div>
                </aside>
            </div>
        </div>
    );
}

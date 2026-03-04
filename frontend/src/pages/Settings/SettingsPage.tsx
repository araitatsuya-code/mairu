import './SettingsPage.css';

import { useState } from 'react';

import {
    checkGmailConnection,
    clearClaudeAPIKey,
    cancelGoogleLogin,
    saveClaudeAPIKey,
    startGoogleLogin,
    type GmailConnectionResult,
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
    const [gmailPending, setGmailPending] = useState(false);
    const [gmailError, setGmailError] = useState<string | null>(null);
    const [lastGmailResult, setLastGmailResult] = useState<GmailConnectionResult | null>(null);
    const [claudeApiKey, setClaudeApiKey] = useState('');
    const [claudePending, setClaudePending] = useState(false);
    const [claudeError, setClaudeError] = useState<string | null>(null);
    const normalizedClaudeApiKey = claudeApiKey.trim();
    const claudeApiKeyBlank = normalizedClaudeApiKey === '';
    const googleStateLabel = status.authorized
        ? 'トークン保存済み'
        : status.googleConfigured
          ? 'ログイン可能'
          : 'Client ID待ち';
    const gmailStateLabel = status.gmailConnected
        ? '接続済み'
        : status.authorized
          ? '確認待ち'
          : 'ログイン待ち';

    async function refreshStatusSafely(fallbackMessage: string): Promise<boolean> {
        try {
            await onStatusRefresh();
            return true;
        } catch (cause) {
            const message =
                cause instanceof Error
                    ? cause.message
                    : fallbackMessage;
            setLoginError(message);
            return false;
        }
    }

    async function handleGoogleLogin() {
        setLoginPending(true);
        setLoginError(null);
        setLastLoginResult(null);
        setLoginNote(null);

        try {
            const result = await startGoogleLogin();
            setLastLoginResult(result);
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
        } finally {
            await refreshStatusSafely('状態の再取得に失敗しました。');
            setLoginPending(false);
        }
    }

    async function handleCancelLogin() {
        setLoginNote('中断しています...');
        setLoginError(null);

        try {
            const cancelled = await cancelGoogleLogin();
            if (!cancelled) {
                setLoginNote('中断対象のログインが見つかりませんでした。');
                return;
            }

            const refreshed = await refreshStatusSafely('状態の再取得に失敗しました。');
            if (refreshed) {
                setLoginNote(null);
            }
        } catch (cause) {
            const message =
                cause instanceof Error
                    ? cause.message
                    : 'ログイン中断に失敗しました。';
            setLoginError(message);
            setLoginNote(null);
        }
    }

    async function handleSaveClaudeAPIKey() {
        const normalized = normalizedClaudeApiKey;
        if (normalized === '') {
            setClaudeError('Claude API キーを入力してください。');
            return;
        }

        setClaudePending(true);
        setClaudeError(null);

        try {
            const result = await saveClaudeAPIKey(normalized);
            if (!result.success) {
                setClaudeError(result.message);
                return;
            }

            setClaudeApiKey('');
            await onStatusRefresh();
        } catch (cause) {
            const message =
                cause instanceof Error
                    ? cause.message
                    : 'Claude API キーの保存に失敗しました。';
            setClaudeError(message);
        } finally {
            setClaudePending(false);
        }
    }

    async function handleClearClaudeAPIKey() {
        setClaudePending(true);
        setClaudeError(null);

        try {
            const result = await clearClaudeAPIKey();
            if (!result.success) {
                setClaudeError(result.message);
                return;
            }

            await onStatusRefresh();
        } catch (cause) {
            const message =
                cause instanceof Error
                    ? cause.message
                    : 'Claude API キーの削除に失敗しました。';
            setClaudeError(message);
        } finally {
            setClaudePending(false);
        }
    }

    async function handleCheckGmailConnection() {
        setGmailPending(true);
        setGmailError(null);
        setLastGmailResult(null);

        try {
            const result = await checkGmailConnection();
            if (!result.success) {
                setGmailError(result.message);
            } else {
                setLastGmailResult(result);
            }

            try {
                await onStatusRefresh();
            } catch (cause) {
                const message =
                    cause instanceof Error
                        ? cause.message
                        : '状態の再取得に失敗しました。';
                setGmailError((previous) => (previous ? `${previous} / ${message}` : message));
            }
        } catch (cause) {
            const message =
                cause instanceof Error
                    ? cause.message
                    : 'Gmail 接続確認に失敗しました。';
            setGmailError(message);
        } finally {
            setGmailPending(false);
        }
    }

    return (
        <div className="settings-page">
            <section className="settings-hero">
                <p className="settings-eyebrow">MAIRU-006 / #6</p>
                <h1>{appName} 設定ハブ</h1>
                <p className="settings-lead">
                    起動直後に必要な初期状態をここで確認し、OAuth、Claude API キー、
                    通知や自動実行の導線を段階的に接続していきます。
                </p>
            </section>

            <section className="settings-status-grid" aria-label="初期状態サマリー">
                <StatusCard
                    label="Google 認証"
                    readyLabel="トークン保存済み"
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
                                リダイレクト受信、トークン交換、OS キーチェーン保存までをこの場で完結させます。
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
                                {status.authorized && status.googleTokenPreview ? (
                                    <p className="settings-inline-note">
                                        再利用用トークンプレビュー: {status.googleTokenPreview}
                                    </p>
                                ) : null}
                                {lastLoginResult ? (
                                    <dl className="settings-result-grid">
                                        <div>
                                            <dt>結果</dt>
                                            <dd>{lastLoginResult.message}</dd>
                                        </div>
                                        <div>
                                            <dt>保存状態</dt>
                                            <dd>{lastLoginResult.tokenStored ? 'キーチェーン保存済み' : '未保存'}</dd>
                                        </div>
                                        <div>
                                            <dt>再利用用トークン</dt>
                                            <dd>
                                                {lastLoginResult.refreshTokenStored
                                                    ? lastLoginResult.storedPreview || '発行済み'
                                                    : '未取得'}
                                            </dd>
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
                        <li className="rounded-[28px] border border-slate-400/10 bg-slate-900/50 p-6 shadow-[0_18px_60px_rgba(15,23,42,0.24)]">
                            <div className="flex flex-wrap items-start justify-between gap-3">
                                <h3 className="text-xl font-semibold text-slate-50">Gmail API 接続確認</h3>
                                <span
                                    className={`inline-flex items-center rounded-full border px-3 py-1 text-[11px] font-bold uppercase tracking-[0.18em] ${
                                        status.gmailConnected
                                            ? 'border-emerald-300/30 bg-emerald-300/10 text-emerald-100'
                                            : 'border-amber-200/20 bg-amber-200/10 text-amber-100'
                                    }`}
                                >
                                    {gmailStateLabel}
                                </span>
                            </div>
                            <p className="mt-3 text-sm leading-7 text-slate-300">
                                保存済みトークンを再利用し、必要なら更新したうえで Gmail プロフィール取得により
                                接続確認を行います。
                            </p>
                            <div className="mt-4 grid gap-3">
                                <div className="flex flex-wrap items-center gap-3">
                                    <button
                                        className="inline-flex items-center justify-center rounded-[14px] bg-sky-300 px-4 py-2.5 text-sm font-bold text-slate-950 transition hover:bg-sky-200 disabled:cursor-not-allowed disabled:opacity-50"
                                        type="button"
                                        onClick={() => {
                                            void handleCheckGmailConnection();
                                        }}
                                        disabled={gmailPending || !status.authorized}
                                    >
                                        {gmailPending ? '接続確認中...' : 'Gmail 接続確認'}
                                    </button>
                                </div>
                                <p className="text-sm leading-7 text-sky-100">{status.gmailStatus}</p>
                                {status.gmailConnected && status.gmailAccountEmail ? (
                                    <p className="text-sm leading-7 text-sky-100">
                                        接続済みアカウント: {status.gmailAccountEmail}
                                    </p>
                                ) : null}
                                {lastGmailResult?.success ? (
                                    <dl className="grid gap-3 rounded-[18px] border border-slate-400/10 bg-slate-950/35 p-4 md:grid-cols-2">
                                        <div className="grid gap-1">
                                            <dt className="text-[11px] font-semibold uppercase tracking-[0.18em] text-slate-400">アカウント</dt>
                                            <dd className="text-sm font-medium text-slate-50">{lastGmailResult.emailAddress}</dd>
                                        </div>
                                        <div className="grid gap-1">
                                            <dt className="text-[11px] font-semibold uppercase tracking-[0.18em] text-slate-400">メール総数</dt>
                                            <dd className="text-sm font-medium text-slate-50">
                                                {lastGmailResult.messagesTotal.toLocaleString('ja-JP')}
                                            </dd>
                                        </div>
                                        <div className="grid gap-1">
                                            <dt className="text-[11px] font-semibold uppercase tracking-[0.18em] text-slate-400">スレッド総数</dt>
                                            <dd className="text-sm font-medium text-slate-50">
                                                {lastGmailResult.threadsTotal.toLocaleString('ja-JP')}
                                            </dd>
                                        </div>
                                        <div className="grid gap-1">
                                            <dt className="text-[11px] font-semibold uppercase tracking-[0.18em] text-slate-400">トークン更新</dt>
                                            <dd className="text-sm font-medium text-slate-50">
                                                {lastGmailResult.tokenRefreshed ? '実施' : '未実施'}
                                            </dd>
                                        </div>
                                    </dl>
                                ) : null}
                                {gmailError ? (
                                    <p className="text-sm leading-7 text-rose-300">{gmailError}</p>
                                ) : null}
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
                            <div className="settings-action-stack mt-1.5 grid gap-3">
                                <label className="settings-field grid gap-2" htmlFor="claude-api-key">
                                    <span className="settings-field-label text-sm font-bold text-slate-300">
                                        Claude API キー
                                    </span>
                                    <input
                                        id="claude-api-key"
                                        className="settings-input w-full rounded-[14px] border border-slate-400/20 bg-slate-950/40 px-3.5 py-2.5 text-slate-50 placeholder:text-slate-500 focus-visible:outline focus-visible:outline-2 focus-visible:outline-offset-2 focus-visible:outline-sky-200"
                                        type="password"
                                        autoComplete="off"
                                        value={claudeApiKey}
                                        onChange={(event) => {
                                            setClaudeApiKey(event.target.value);
                                        }}
                                        placeholder="sk-ant-api03-..."
                                    />
                                </label>
                                <div className="settings-action-row flex flex-wrap items-center gap-3">
                                    <button
                                        className="settings-action-button inline-flex items-center justify-center rounded-[14px] px-4 py-2.5 font-bold disabled:cursor-not-allowed disabled:opacity-50"
                                        type="button"
                                        onClick={() => {
                                            void handleSaveClaudeAPIKey();
                                        }}
                                        disabled={claudePending || claudeApiKeyBlank}
                                    >
                                        {claudePending ? '保存中...' : 'キーチェーンへ保存'}
                                    </button>
                                    {status.claudeConfigured ? (
                                        <button
                                            className="settings-cancel-button inline-flex items-center justify-center rounded-[14px] px-3.5 py-2.5 font-bold disabled:cursor-not-allowed disabled:opacity-50"
                                            type="button"
                                            onClick={() => {
                                                void handleClearClaudeAPIKey();
                                            }}
                                            disabled={claudePending}
                                        >
                                            保存済みキーを削除
                                        </button>
                                    ) : null}
                                </div>
                                <p className="settings-inline-note text-sm leading-7 text-sky-100">
                                    {status.claudeStatus}
                                </p>
                                {status.claudeConfigured && status.claudeKeyPreview ? (
                                    <p className="settings-inline-note text-sm leading-7 text-sky-100">
                                        保存済みキープレビュー: {status.claudeKeyPreview}
                                    </p>
                                ) : null}
                                {claudeError ? (
                                    <p className="settings-error-note text-sm leading-7 text-rose-300">
                                        {claudeError}
                                    </p>
                                ) : null}
                            </div>
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

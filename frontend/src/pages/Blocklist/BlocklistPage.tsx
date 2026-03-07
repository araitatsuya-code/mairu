import './BlocklistPage.css';

import { useEffect, useState, type FormEvent } from 'react';

import {
    deleteBlocklistEntry,
    loadBlocklistEntries,
    loadBlocklistSuggestions,
    type BlocklistEntry,
    type BlocklistKind,
    type BlocklistSuggestion,
    type RuntimeStatus,
    upsertBlocklistEntry,
} from '../../lib/runtime';

type BlocklistPageProps = {
    status: RuntimeStatus;
};

function kindLabel(kind: BlocklistKind): string {
    switch (kind) {
    case 'sender':
        return '送信者';
    case 'domain':
        return 'ドメイン';
    default:
        return kind;
    }
}

function formatDate(value: string): string {
    const parsed = new Date(value);
    if (Number.isNaN(parsed.getTime())) {
        return value;
    }
    return new Intl.DateTimeFormat('ja-JP', {
        dateStyle: 'medium',
        timeStyle: 'short',
    }).format(parsed);
}

export function BlocklistPage({ status }: BlocklistPageProps) {
    const [entries, setEntries] = useState<BlocklistEntry[]>([]);
    const [suggestions, setSuggestions] = useState<BlocklistSuggestion[]>([]);
    const [kind, setKind] = useState<BlocklistKind>('sender');
    const [pattern, setPattern] = useState('');
    const [note, setNote] = useState('');
    const [loading, setLoading] = useState(true);
    const [saving, setSaving] = useState(false);
    const [message, setMessage] = useState<string | null>(null);
    const [error, setError] = useState<string | null>(null);

    async function refresh() {
        if (!status.databaseReady) {
            setEntries([]);
            setSuggestions([]);
            return;
        }

        const [nextEntries, nextSuggestions] = await Promise.all([
            loadBlocklistEntries(),
            loadBlocklistSuggestions(),
        ]);
        setEntries(nextEntries);
        setSuggestions(nextSuggestions);
    }

    useEffect(() => {
        let cancelled = false;

        async function initialize() {
            setLoading(true);
            setError(null);
            if (!status.databaseReady) {
                setEntries([]);
                setSuggestions([]);
                setLoading(false);
                return;
            }
            try {
                const [nextEntries, nextSuggestions] = await Promise.all([
                    loadBlocklistEntries(),
                    loadBlocklistSuggestions(),
                ]);
                if (cancelled) {
                    return;
                }
                setEntries(nextEntries);
                setSuggestions(nextSuggestions);
            } catch (cause) {
                if (cancelled) {
                    return;
                }
                setError(cause instanceof Error ? cause.message : 'ブロックリスト読み込みに失敗しました。');
            } finally {
                if (!cancelled) {
                    setLoading(false);
                }
            }
        }

        void initialize();

        return () => {
            cancelled = true;
        };
    }, [status.databaseReady]);

    async function handleSubmit(event: FormEvent<HTMLFormElement>) {
        event.preventDefault();
        if (!pattern.trim()) {
            setError('パターンを入力してください。');
            return;
        }

        setSaving(true);
        setError(null);
        setMessage(null);
        try {
            const result = await upsertBlocklistEntry({
                kind,
                pattern: pattern.trim(),
                note: note.trim(),
            });
            if (!result.success) {
                setError(result.message);
                return;
            }
            setMessage(result.message);
            setPattern('');
            setNote('');
            await refresh();
        } catch (cause) {
            setError(cause instanceof Error ? cause.message : 'ブロックリスト保存に失敗しました。');
        } finally {
            setSaving(false);
        }
    }

    async function handleDelete(id: number) {
        if (!window.confirm('このブロック設定を削除しますか？')) {
            return;
        }
        setSaving(true);
        setError(null);
        setMessage(null);
        try {
            const result = await deleteBlocklistEntry(id);
            if (!result.success) {
                setError(result.message);
                return;
            }
            setMessage(result.message);
            await refresh();
        } catch (cause) {
            setError(cause instanceof Error ? cause.message : 'ブロックリスト削除に失敗しました。');
        } finally {
            setSaving(false);
        }
    }

    async function handleApplySuggestion(suggestion: BlocklistSuggestion) {
        setSaving(true);
        setError(null);
        setMessage(null);
        try {
            const result = await upsertBlocklistEntry({
                kind: suggestion.kind,
                pattern: suggestion.pattern,
                note: `修正履歴提案 (${suggestion.count} 回)`,
            });
            if (!result.success) {
                setError(result.message);
                return;
            }
            setMessage(result.message);
            await refresh();
        } catch (cause) {
            setError(cause instanceof Error ? cause.message : '提案の追加に失敗しました。');
        } finally {
            setSaving(false);
        }
    }

    return (
        <div className="blocklist-page">
            <section className="blocklist-hero">
                <p className="blocklist-eyebrow">MAIRU-011 / #11</p>
                <h1>ブロックリスト管理</h1>
                <p>
                    既知の不要送信者を sender / domain で登録し、Claude 分析前にスキップできるようにします。
                </p>
                <div className="blocklist-chips">
                    <span className={`blocklist-chip ${status.databaseReady ? 'ready' : 'pending'}`}>
                        SQLite: {status.databaseReady ? '利用可能' : '未初期化'}
                    </span>
                    <span className={`blocklist-chip ${status.claudeConfigured ? 'ready' : 'pending'}`}>
                        Claude API: {status.claudeConfigured ? '設定済み' : '未設定'}
                    </span>
                </div>
            </section>

            <section className="blocklist-card">
                <h2>手動追加</h2>
                <form className="blocklist-form" onSubmit={(event) => void handleSubmit(event)}>
                    <label>
                        <span>種別</span>
                        <select
                            value={kind}
                            onChange={(event) => {
                                setKind(event.target.value as BlocklistKind);
                            }}
                            disabled={saving || loading}
                        >
                            <option value="sender">送信者メールアドレス</option>
                            <option value="domain">ドメイン</option>
                        </select>
                    </label>
                    <label>
                        <span>パターン</span>
                        <input
                            type="text"
                            value={pattern}
                            onChange={(event) => {
                                setPattern(event.target.value);
                            }}
                            placeholder={kind === 'sender' ? 'noreply@example.com' : 'example.com'}
                            disabled={saving || loading}
                        />
                    </label>
                    <label>
                        <span>メモ (任意)</span>
                        <input
                            type="text"
                            value={note}
                            onChange={(event) => {
                                setNote(event.target.value);
                            }}
                            placeholder="例: メルマガ系を恒久的に除外"
                            disabled={saving || loading}
                        />
                    </label>
                    <button type="submit" disabled={saving || loading || !status.databaseReady}>
                        {saving ? '保存中...' : 'ブロックに追加'}
                    </button>
                </form>
                {!status.databaseReady ? (
                    <p className="blocklist-note">SQLite 初期化が完了すると保存できます。</p>
                ) : null}
                {message ? <p className="blocklist-note">{message}</p> : null}
                {error ? <p className="blocklist-error">{error}</p> : null}
            </section>

            <section className="blocklist-card">
                <h2>修正履歴からの提案</h2>
                {loading ? <p className="blocklist-note">提案を読み込み中です...</p> : null}
                {!loading && suggestions.length === 0 ? (
                    <p className="blocklist-note">提案はまだありません（同一送信者/ドメイン 3 回以上で表示）。</p>
                ) : null}
                {suggestions.length > 0 ? (
                    <ul className="blocklist-suggestions">
                        {suggestions.map((suggestion) => (
                            <li key={`${suggestion.kind}-${suggestion.pattern}`}>
                                <div>
                                    <strong>{kindLabel(suggestion.kind)}: </strong>
                                    <span>{suggestion.pattern}</span>
                                    <p>{suggestion.description}</p>
                                    <small>最終修正: {formatDate(suggestion.lastSeenAt)}</small>
                                </div>
                                <button
                                    type="button"
                                    onClick={() => {
                                        void handleApplySuggestion(suggestion);
                                    }}
                                    disabled={saving || loading || !status.databaseReady}
                                >
                                    追加
                                </button>
                            </li>
                        ))}
                    </ul>
                ) : null}
            </section>

            <section className="blocklist-card">
                <h2>登録済み一覧</h2>
                {loading ? (
                    <p className="blocklist-note">ブロックリストを読み込み中です...</p>
                ) : entries.length === 0 ? (
                    <p className="blocklist-note">登録済みブロックはありません。</p>
                ) : (
                    <div className="blocklist-table-wrap">
                        <table>
                            <thead>
                                <tr>
                                    <th scope="col">種別</th>
                                    <th scope="col">パターン</th>
                                    <th scope="col">メモ</th>
                                    <th scope="col">更新日時</th>
                                    <th scope="col">操作</th>
                                </tr>
                            </thead>
                            <tbody>
                                {entries.map((entry) => (
                                    <tr key={entry.id}>
                                        <td>{kindLabel(entry.kind)}</td>
                                        <td>{entry.pattern}</td>
                                        <td>{entry.note || '-'}</td>
                                        <td>{formatDate(entry.updatedAt)}</td>
                                        <td>
                                            <button
                                                type="button"
                                                onClick={() => {
                                                    void handleDelete(entry.id);
                                                }}
                                                disabled={saving || loading}
                                            >
                                                削除
                                            </button>
                                        </td>
                                    </tr>
                                ))}
                            </tbody>
                        </table>
                    </div>
                )}
            </section>
        </div>
    );
}

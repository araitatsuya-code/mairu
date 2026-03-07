import './App.css';
import { useEffect, useState } from 'react';

import {
    defaultRuntimeStatus,
    loadAppName,
    loadRuntimeStatus,
    type RuntimeStatus,
} from './lib/runtime';
import { BlocklistPage } from './pages/Blocklist/BlocklistPage';
import { ClassifyPage } from './pages/Classify/ClassifyPage';
import { ExportPage } from './pages/Export/ExportPage';
import { SettingsPage } from './pages/Settings/SettingsPage';

type AppView = 'settings' | 'classify' | 'blocklist' | 'export';

function App() {
    const [appName, setAppName] = useState('Mairu');
    const [status, setStatus] = useState<RuntimeStatus>(defaultRuntimeStatus);
    const [loading, setLoading] = useState(true);
    const [error, setError] = useState<string | null>(null);
    const [view, setView] = useState<AppView>('settings');

    async function refreshRuntimeStatus(): Promise<RuntimeStatus> {
        const nextStatus = await loadRuntimeStatus();
        setStatus(nextStatus);
        return nextStatus;
    }

    useEffect(() => {
        let cancelled = false;

        async function initialize() {
            try {
                const [nextAppName, nextStatus] = await Promise.all([loadAppName(), loadRuntimeStatus()]);

                if (cancelled) {
                    return;
                }

                setAppName(nextAppName);
                setStatus(nextStatus);
            } catch (cause) {
                if (cancelled) {
                    return;
                }

                const message =
                    cause instanceof Error
                        ? cause.message
                        : '初期設定の読み込みに失敗しました。';
                setError(message);
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
    }, []);

    return (
        <div className="app-shell">
            <main className="app-frame">
                {loading ? (
                    <section className="app-loading">
                        <h1>初期設定を確認しています</h1>
                        <p>起動時に必要な状態を読み込み、Settings 画面を準備しています。</p>
                    </section>
                ) : error ? (
                    <section className="app-error">
                        <h1>初期化に失敗しました</h1>
                        <p>{error}</p>
                    </section>
                ) : (
                    <div className="app-content">
                        <nav className="app-nav" aria-label="ページ切り替え">
                            <button
                                className={`app-nav-button ${view === 'settings' ? 'active' : ''}`}
                                type="button"
                                aria-pressed={view === 'settings'}
                                aria-current={view === 'settings' ? 'page' : undefined}
                                onClick={() => {
                                    setView('settings');
                                }}
                            >
                                Settings
                            </button>
                            <button
                                className={`app-nav-button ${view === 'classify' ? 'active' : ''}`}
                                type="button"
                                aria-pressed={view === 'classify'}
                                aria-current={view === 'classify' ? 'page' : undefined}
                                onClick={() => {
                                    setView('classify');
                                }}
                            >
                                Classify
                            </button>
                            <button
                                className={`app-nav-button ${view === 'blocklist' ? 'active' : ''}`}
                                type="button"
                                aria-pressed={view === 'blocklist'}
                                aria-current={view === 'blocklist' ? 'page' : undefined}
                                onClick={() => {
                                    setView('blocklist');
                                }}
                            >
                                Blocklist
                            </button>
                            <button
                                className={`app-nav-button ${view === 'export' ? 'active' : ''}`}
                                type="button"
                                aria-pressed={view === 'export'}
                                aria-current={view === 'export' ? 'page' : undefined}
                                onClick={() => {
                                    setView('export');
                                }}
                            >
                                Export
                            </button>
                        </nav>
                        {view === 'settings' ? (
                            <SettingsPage
                                appName={appName}
                                status={status}
                                onStatusRefresh={refreshRuntimeStatus}
                            />
                        ) : view === 'classify' ? (
                            <ClassifyPage status={status} />
                        ) : view === 'export' ? (
                            <ExportPage status={status} />
                        ) : (
                            <BlocklistPage status={status} />
                        )}
                    </div>
                )}
            </main>
        </div>
    );
}

export default App;

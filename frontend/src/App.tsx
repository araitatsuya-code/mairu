import './App.css';
import { useEffect, useState } from 'react';

import {
    defaultRuntimeStatus,
    loadAppName,
    loadRuntimeStatus,
    type RuntimeStatus,
} from './lib/runtime';
import { ClassifyPage } from './pages/Classify/ClassifyPage';
import { SettingsPage } from './pages/Settings/SettingsPage';

type AppPage = 'classify' | 'settings';

function App() {
    const [appName, setAppName] = useState('Mairu');
    const [status, setStatus] = useState<RuntimeStatus>(defaultRuntimeStatus);
    const [loading, setLoading] = useState(true);
    const [error, setError] = useState<string | null>(null);
    const [activePage, setActivePage] = useState<AppPage>('classify');

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
                    <>
                        <nav className="app-nav" aria-label="ページ切り替え">
                            <button
                                type="button"
                                className={`app-nav-button ${activePage === 'classify' ? 'active' : ''}`}
                                onClick={() => {
                                    setActivePage('classify');
                                }}
                            >
                                分類確認
                            </button>
                            <button
                                type="button"
                                className={`app-nav-button ${activePage === 'settings' ? 'active' : ''}`}
                                onClick={() => {
                                    setActivePage('settings');
                                }}
                            >
                                Settings
                            </button>
                        </nav>
                        {activePage === 'classify' ? (
                            <ClassifyPage />
                        ) : (
                            <SettingsPage
                                appName={appName}
                                status={status}
                                onStatusRefresh={refreshRuntimeStatus}
                            />
                        )}
                    </>
                )}
            </main>
        </div>
    );
}

export default App;

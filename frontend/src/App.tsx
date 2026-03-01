import './App.css';

const bootstrapSteps = [
    'Wails の基本構成を初期化済み',
    'React + TypeScript のフロントエンドを配置済み',
    '次は Settings 画面と OAuth 導線を実装する',
];

function App() {
    return (
        <div className="app-shell">
            <main className="hero-card">
                <p className="eyebrow">MAIRU-001 / #1</p>
                <h1>Mairu 開発ブートストラップ</h1>
                <p className="lead">
                    Wails と React の土台を作成しました。Gmail 整理アプリの実装は、
                    この土台の上に段階的に積み上げます。
                </p>
                <section className="status-panel">
                    <h2>現在の状態</h2>
                    <ul className="step-list">
                        {bootstrapSteps.map((step) => (
                            <li key={step}>{step}</li>
                        ))}
                    </ul>
                </section>
            </main>
        </div>
    );
}

export default App;

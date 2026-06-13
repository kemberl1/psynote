// Стартовая страница-заглушка (Этап 1 каркаса).
// Полноценный UI (опросник, рабочая область, история, экспорт) — Этапы 4–7,
// дизайн в стиле cursor.com — см. docs/08_ui_ux.md.

function App() {
  return (
    <main className="app">
      <div className="card">
        <div className="badge">RAG · Психиатрические дневники</div>
        <h1 className="title">
          AI MED — <span className="accent">генерация дневников</span>
        </h1>
        <p className="subtitle">
          Каркас проекта (Этап 1). Сервисы поднимаются через docker-compose:
          frontend, gateway (Go), rag (Python), postgres, qdrant.
        </p>
        <ul className="status-list">
          <li>
            <span className="dot" /> Frontend — React 19 + Vite + TypeScript
          </li>
          <li>
            <span className="dot" /> Gateway — Go (API · анонимизация · экспорт)
          </li>
          <li>
            <span className="dot" /> RAG — Python · FastAPI · Qdrant
          </li>
        </ul>
        <p className="footnote">
          Бизнес-логика появится на следующих этапах (см. docs/10_roadmap_stepbystep.md).
        </p>
      </div>
    </main>
  );
}

export default App;

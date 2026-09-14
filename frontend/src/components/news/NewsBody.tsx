import type { NewsInline } from "../../lib/newsMarkup";
import { parseNewsMarkup } from "../../lib/newsMarkup";

function Inlines({ items }: { items: NewsInline[] }) {
  return (
    <>
      {items.map((run, i) => {
        if (run.kind === "bold") return <strong key={i}>{run.text}</strong>;
        if (run.kind === "code") return <code key={i}>{run.text}</code>;
        return <span key={i}>{run.text}</span>;
      })}
    </>
  );
}

export function NewsBody({ source }: { source: string }) {
  const blocks = parseNewsMarkup(source);
  if (blocks.length === 0) {
    return <p className="news-prose__empty">Пока пусто</p>;
  }
  return (
    <div className="news-prose">
      {blocks.map((block, i) => {
        if (block.type === "h2") return <h2 key={i}>{block.text}</h2>;
        if (block.type === "h3") return <h3 key={i}>{block.text}</h3>;
        if (block.type === "ul") {
          return (
            <ul key={i}>
              {block.items.map((item, j) => (
                <li key={j}>
                  <Inlines items={item} />
                </li>
              ))}
            </ul>
          );
        }
        return (
          <p key={i}>
            <Inlines items={block.inlines} />
          </p>
        );
      })}
    </div>
  );
}

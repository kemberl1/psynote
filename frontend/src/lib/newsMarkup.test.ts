import { describe, expect, it } from "vitest";
import { parseNewsInlines, parseNewsMarkup } from "./newsMarkup";

describe("parseNewsInlines", () => {
  it("keeps plain text", () => {
    expect(parseNewsInlines("просто текст")).toEqual([
      { kind: "text", text: "просто текст" },
    ]);
  });

  it("extracts bold and code", () => {
    expect(parseNewsInlines("есть **жирный** и `код`")).toEqual([
      { kind: "text", text: "есть " },
      { kind: "bold", text: "жирный" },
      { kind: "text", text: " и " },
      { kind: "code", text: "код" },
    ]);
  });
});

describe("parseNewsMarkup", () => {
  it("parses headings, lists and paragraphs", () => {
    const blocks = parseNewsMarkup(
      "## Что нового\n\nКоротко про **пакет**.\n\n- один\n- два\n\n### Дальше\nхвост",
    );
    expect(blocks[0]).toEqual({ type: "h2", text: "Что нового" });
    expect(blocks[1]).toMatchObject({ type: "p" });
    expect(blocks[2]).toMatchObject({ type: "ul" });
    expect(blocks[2].type === "ul" && blocks[2].items).toHaveLength(2);
    expect(blocks[3]).toEqual({ type: "h3", text: "Дальше" });
    expect(blocks[4]).toMatchObject({ type: "p" });
  });

  it("joins wrapped paragraph lines", () => {
    const blocks = parseNewsMarkup("первая строка\nвторая строка");
    expect(blocks).toHaveLength(1);
    expect(blocks[0]).toMatchObject({ type: "p" });
    if (blocks[0].type === "p") {
      expect(blocks[0].inlines[0]).toEqual({
        kind: "text",
        text: "первая строка вторая строка",
      });
    }
  });

  it("returns empty for blank input", () => {
    expect(parseNewsMarkup("  \n\n")).toEqual([]);
  });
});

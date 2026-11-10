// @vitest-environment node
import { ESLint } from "eslint";
import { fileURLToPath } from "node:url";
import { describe, expect, it } from "vitest";

// Страховка ADR-008: правило i18next/no-literal-string действительно ловит текст в JSX.
// Если обновление плагина или ESLint тихо выключит его, упадёт этот тест, а не ревью.
const cwd = fileURLToPath(new URL("../..", import.meta.url));
const eslint = new ESLint({ cwd });

const literalErrors = async (code: string) => {
  const [result] = await eslint.lintText(code, { filePath: `${cwd}/src/Probe.tsx` });
  return (result?.messages ?? []).filter((m) => m.ruleId === "i18next/no-literal-string");
};

describe("правило no-literal-string", () => {
  it("ловит текст между тегами", async () => {
    const errors = await literalErrors(
      "export const Probe = () => (\n  <div>\n    <p>Привет мир</p>\n  </div>\n);\n",
    );
    expect(errors).toHaveLength(1);
  });

  it("не ругается на translate() и строковые пропсы", async () => {
    const errors = await literalErrors(
      'import { useTranslate } from "react-admin";\n' +
        "export const Probe = () => {\n" +
        "  const translate = useTranslate();\n" +
        '  return <p className="x">{translate("aim.app.title")}</p>;\n' +
        "};\n",
    );
    expect(errors).toHaveLength(0);
  });
});

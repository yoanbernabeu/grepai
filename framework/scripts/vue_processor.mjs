import path from "node:path";
import { createRequire } from "node:module";
import { pathToFileURL } from "node:url";

const chunks = [];
for await (const chunk of process.stdin) {
  chunks.push(chunk);
}

const input = JSON.parse(Buffer.concat(chunks).toString("utf8"));
const source = input.source ?? "";
const filePath = input.filePath ?? "Component.vue";
const absoluteFilePath = path.isAbsolute(filePath) ? filePath : path.resolve(process.cwd(), filePath);

function candidateResolveDirs(startFilePath) {
  const dirs = [];
  let dir = path.dirname(startFilePath);
  while (true) {
    dirs.push(dir);
    const parent = path.dirname(dir);
    if (parent === dir) break;
    dir = parent;
  }
  if (!dirs.includes(process.cwd())) {
    dirs.push(process.cwd());
  }
  return dirs;
}

async function loadVueCompiler(startFilePath) {
  for (const dir of candidateResolveDirs(startFilePath)) {
    const req = createRequire(path.join(dir, "__grepai_vue_processor__.cjs"));
    try {
      const resolved = req.resolve("@vue/compiler-sfc");
      return import(pathToFileURL(resolved).href);
    } catch (err) {
      if (err?.code !== "MODULE_NOT_FOUND") {
        throw err;
      }
    }
  }
  throw new Error(`Cannot resolve @vue/compiler-sfc from ${path.dirname(startFilePath)}`);
}

function countLines(text) {
  if (text.length === 0) return 1;
  return text.split("\n").length;
}

function pushMapped(out, map, text, sourceStartLine) {
  if (!text) return;
  out.push(text);
  const lineCount = countLines(text);
  for (let i = 0; i < lineCount; i++) {
    map.push(sourceStartLine + i);
  }
}

function pushUnmapped(out, map, text) {
  if (!text) return;
  out.push(text);
  const lineCount = countLines(text);
  for (let i = 0; i < lineCount; i++) {
    map.push(0);
  }
}

function normalizeExpr(expr) {
  const trimmed = expr.trim();
  const singleQuoted = /^'([^']+)'$/.exec(trimmed);
  if (singleQuoted) return singleQuoted[1];
  const doubleQuoted = /^"([^"]+)"$/.exec(trimmed);
  if (doubleQuoted) return doubleQuoted[1];
  return trimmed;
}

function extractVBindExprsFromLine(line) {
  const out = [];
  let start = 0;
  while (start < line.length) {
    const idx = line.indexOf("v-bind(", start);
    if (idx < 0) break;
    let i = idx + "v-bind(".length;
    let depth = 1;
    while (i < line.length && depth > 0) {
      const ch = line[i];
      if (ch === "(") depth++;
      else if (ch === ")") depth--;
      i++;
    }
    if (depth === 0) {
      const expr = line.slice(idx + "v-bind(".length, i - 1);
      out.push(normalizeExpr(expr));
      start = i;
    } else {
      break;
    }
  }
  return out;
}

function extractStyleVBindExpressions(styleContent, styleStartLine) {
  const refs = [];
  const lines = styleContent.split("\n");
  for (let i = 0; i < lines.length; i++) {
    const exprs = extractVBindExprsFromLine(lines[i]);
    for (const expr of exprs) {
      refs.push({
        expr,
        line: styleStartLine + i,
      });
    }
  }
  return refs;
}

function appendStyleBindings(out, map, refs, index) {
  if (!refs.length) return;
  out.push(`function __vue_style_bindings__${index}() {`);
  map.push(refs[0].line);
  for (const ref of refs) {
    out.push(`  __css_v_bind__(${ref.expr});`);
    map.push(ref.line);
  }
  out.push("}");
  map.push(refs[refs.length - 1].line);
}

try {
  const { parse, compileTemplate } = await loadVueCompiler(absoluteFilePath);
  const parsed = parse(source, { filename: filePath });
  if (parsed.errors?.length) {
    const msg = String(parsed.errors[0]);
    throw new Error(msg);
  }

  const d = parsed.descriptor;
  const out = [];
  const map = [];

  if (d.script?.content) {
    pushMapped(out, map, d.script.content, d.script.loc.start.line + 1);
  }
  if (d.scriptSetup?.content) {
    if (out.length > 0) {
      out.push("\n");
      map.push(0);
    }
    pushMapped(out, map, d.scriptSetup.content, d.scriptSetup.loc.start.line + 1);
  }

  if (d.template?.content) {
    const compiled = compileTemplate({
      id: filePath,
      source: d.template.content,
      filename: filePath,
      scoped: Boolean(d.styles?.some((s) => s.scoped)),
    });
    if (compiled.errors?.length) {
      const first = compiled.errors[0];
      throw new Error(typeof first === "string" ? first : first.message || String(first));
    }
    if (compiled.code) {
      if (out.length > 0) {
        out.push("\n");
        map.push(0);
      }
      pushUnmapped(out, map, compiled.code);
    }
  }

  if (Array.isArray(d.styles) && d.styles.length > 0) {
    for (let i = 0; i < d.styles.length; i++) {
      const styleBlock = d.styles[i];
      if (!styleBlock?.content) continue;
      if (out.length > 0) {
        out.push("\n");
        map.push(0);
      }
      const styleStartLine = styleBlock.loc?.start?.line ? styleBlock.loc.start.line + 1 : 1;
      // Include raw style content for semantic search.
      pushMapped(out, map, styleBlock.content, styleStartLine);

      const refs = extractStyleVBindExpressions(styleBlock.content, styleStartLine);
      if (refs.length > 0) {
        out.push("\n");
        map.push(0);
        appendStyleBindings(out, map, refs, i);
      }
    }
  }

  const text = out.join("\n");
  process.stdout.write(
    JSON.stringify({
      embeddingText: text,
      traceText: text,
      virtualPath: `${filePath}.__trace__.ts`,
      generatedToSourceLine: map,
      warnings: [],
    }),
  );
} catch (err) {
  process.stderr.write(String(err?.stack || err?.message || err));
  process.exit(1);
}

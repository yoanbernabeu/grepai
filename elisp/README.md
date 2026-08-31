# tree-sitter-elisp (vendored)

Vendored copy of [Wilfred/tree-sitter-elisp](https://github.com/Wilfred/tree-sitter-elisp).

- **Source:** https://github.com/Wilfred/tree-sitter-elisp
- **Commit:** `3a8f590258404ae955d46149edaaa5028078c6bc`
- **License:** MIT (see `LICENSE`)

The `smacker/go-tree-sitter` repo does not ship an elisp grammar, so we
vendor here following the same pattern as `fsharp/` (which vendored the
Ionide tree-sitter-fsharp grammar).

## Re-vendoring

To pull a newer commit:

```bash
cd $(mktemp -d) && git clone --depth=1 https://github.com/Wilfred/tree-sitter-elisp
cp tree-sitter-elisp/src/parser.c          PATH/TO/grepai/elisp/parser.c
cp tree-sitter-elisp/src/tree_sitter/parser.h PATH/TO/grepai/elisp/tree_sitter/parser.h
cp tree-sitter-elisp/LICENSE                PATH/TO/grepai/elisp/LICENSE
# update the commit SHA in this README
```

No `scanner.c` is needed — this grammar is parser-only.

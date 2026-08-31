package elisp

//#include "tree_sitter/parser.h"
//TSLanguage *tree_sitter_elisp();
import "C"
import (
	"unsafe"

	sitter "github.com/smacker/go-tree-sitter"
)

// GetLanguage returns the tree-sitter grammar for Emacs Lisp.
// Grammar source: https://github.com/Wilfred/tree-sitter-elisp (MIT license).
func GetLanguage() *sitter.Language {
	ptr := unsafe.Pointer(C.tree_sitter_elisp())
	return sitter.NewLanguage(ptr)
}

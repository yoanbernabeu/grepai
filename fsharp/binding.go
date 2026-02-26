package fsharp

//#include "tree_sitter/parser.h"
//TSLanguage *tree_sitter_fsharp();
import "C"
import (
	"unsafe"

	sitter "github.com/smacker/go-tree-sitter"
)

// GetLanguage returns the tree-sitter grammar for F#.
// Grammar source: https://github.com/ionide/tree-sitter-fsharp (MIT license)
func GetLanguage() *sitter.Language {
	ptr := unsafe.Pointer(C.tree_sitter_fsharp())
	return sitter.NewLanguage(ptr)
}

package framework

import (
	"path/filepath"
	"strings"
)

func hasExt(filePath, ext string) bool {
	return strings.EqualFold(filepath.Ext(filePath), ext)
}

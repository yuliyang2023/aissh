package i18n

import (
	"go/ast"
	"go/parser"
	"go/token"
	"io/fs"
	"path/filepath"
	"strconv"
	"strings"
	"testing"
	"unicode"
)

// Missing entries silently fall back to Chinese at runtime. Check literal UI
// message IDs at their call sites so new translations cannot be forgotten.
func TestEnglishCatalogCoversLocalizedMessages(t *testing.T) {
	fset := token.NewFileSet()
	for _, root := range []string{"..", "../../cmd"} {
		err := filepath.WalkDir(root, func(path string, entry fs.DirEntry, err error) error {
			if err != nil {
				return err
			}
			if entry.IsDir() || !strings.HasSuffix(path, ".go") || strings.HasSuffix(path, "_test.go") {
				return nil
			}
			file, err := parser.ParseFile(fset, path, nil, 0)
			if err != nil {
				return err
			}
			ast.Inspect(file, func(node ast.Node) bool {
				call, ok := node.(*ast.CallExpr)
				if !ok || len(call.Args) == 0 {
					return true
				}
				selector, ok := call.Fun.(*ast.SelectorExpr)
				if !ok || selector.Sel.Name != "t" && selector.Sel.Name != "T" {
					return true
				}
				literal, ok := call.Args[0].(*ast.BasicLit)
				if !ok || literal.Kind != token.STRING {
					return true
				}
				message, err := strconv.Unquote(literal.Value)
				if err != nil {
					t.Fatal(err)
				}
				if strings.IndexFunc(message, func(r rune) bool { return unicode.Is(unicode.Han, r) }) < 0 {
					return true
				}
				if translation, ok := englishCatalog[message]; !ok || translation == "" {
					t.Errorf("%s: missing English translation for %q", fset.Position(literal.Pos()), message)
				}
				return true
			})
			return nil
		})
		if err != nil {
			t.Fatal(err)
		}
	}
}

package notebooklm_test

import (
	"go/token"
	"go/types"
	"os"
	"os/exec"
	"strings"
	"testing"

	"golang.org/x/tools/go/gcexportdata"
)

func TestPublicSurfaceHasNoInternalTypes(t *testing.T) {
	const packagePath = "github.com/tmc/nlm/notebooklm"
	cmd := exec.Command("go", "list", "-export", "-f={{.Export}}", packagePath)
	cmd.Env = append(os.Environ(), "GOWORK=off")
	output, err := cmd.Output()
	if err != nil {
		t.Fatal(err)
	}
	file, err := os.Open(strings.TrimSpace(string(output)))
	if err != nil {
		t.Fatal(err)
	}
	defer file.Close()
	reader, err := gcexportdata.NewReader(file)
	if err != nil {
		t.Fatal(err)
	}
	pkg, err := gcexportdata.Read(reader, token.NewFileSet(), make(map[string]*types.Package), packagePath)
	if err != nil {
		t.Fatal(err)
	}

	check := publicTypeChecker{
		t:           t,
		packagePath: packagePath,
		seen:        make(map[types.Type]bool),
	}
	scope := pkg.Scope()
	for _, name := range scope.Names() {
		if token.IsExported(name) {
			check.object(scope.Lookup(name))
		}
	}
}

type publicTypeChecker struct {
	t           *testing.T
	packagePath string
	seen        map[types.Type]bool
}

func (c *publicTypeChecker) object(object types.Object) {
	c.t.Helper()
	c.typ(object.Type())
}

func (c *publicTypeChecker) typ(value types.Type) {
	c.t.Helper()
	if value == nil || c.seen[value] {
		return
	}
	c.seen[value] = true

	switch value := value.(type) {
	case *types.Alias:
		c.typ(types.Unalias(value))
	case *types.Array:
		c.typ(value.Elem())
	case *types.Chan:
		c.typ(value.Elem())
	case *types.Interface:
		for i := 0; i < value.NumMethods(); i++ {
			method := value.Method(i)
			if method.Exported() {
				c.object(method)
			}
		}
		for i := 0; i < value.NumEmbeddeds(); i++ {
			c.typ(value.EmbeddedType(i))
		}
	case *types.Map:
		c.typ(value.Key())
		c.typ(value.Elem())
	case *types.Named:
		object := value.Obj()
		if object.Pkg() != nil {
			path := object.Pkg().Path()
			if strings.Contains(path, "/internal/") {
				c.t.Errorf("public API refers to internal type %s", value)
				return
			}
			if path != c.packagePath {
				return
			}
		}
		c.typ(value.Underlying())
		c.methods(value)
		c.methods(types.NewPointer(value))
	case *types.Pointer:
		c.typ(value.Elem())
	case *types.Signature:
		c.tuple(value.Params())
		c.tuple(value.Results())
		if value.RecvTypeParams() != nil {
			c.typeParams(value.RecvTypeParams())
		}
		if value.TypeParams() != nil {
			c.typeParams(value.TypeParams())
		}
	case *types.Slice:
		c.typ(value.Elem())
	case *types.Struct:
		for i := 0; i < value.NumFields(); i++ {
			field := value.Field(i)
			if field.Exported() {
				c.object(field)
			}
		}
	case *types.TypeParam:
		c.typ(value.Constraint())
	case *types.Union:
		for i := 0; i < value.Len(); i++ {
			c.typ(value.Term(i).Type())
		}
	}
}

func (c *publicTypeChecker) methods(value types.Type) {
	c.t.Helper()
	set := types.NewMethodSet(value)
	for i := 0; i < set.Len(); i++ {
		method := set.At(i)
		if method.Obj().Exported() {
			c.object(method.Obj())
		}
	}
}

func (c *publicTypeChecker) tuple(tuple *types.Tuple) {
	c.t.Helper()
	for i := 0; i < tuple.Len(); i++ {
		c.typ(tuple.At(i).Type())
	}
}

func (c *publicTypeChecker) typeParams(params *types.TypeParamList) {
	c.t.Helper()
	for i := 0; i < params.Len(); i++ {
		c.typ(params.At(i))
	}
}

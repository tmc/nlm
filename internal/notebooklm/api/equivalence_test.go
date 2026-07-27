package api

import (
	"fmt"
	"reflect"
	"strings"
	"testing"
)

type equivalenceMismatch struct {
	Phase string
	Path  string
	Old   string
	New   string
}

// assertEquivalent compares the typed legacy and proto projections without
// hiding a mismatch behind a top-level DeepEqual. It is deliberately strict:
// callers must normalize only differences that are part of their public
// contract before invoking it.
func assertEquivalent(t *testing.T, phase string, old, new any) {
	t.Helper()
	mismatches := compareEquivalent(reflect.ValueOf(old), reflect.ValueOf(new), "$")
	for i := range mismatches {
		mismatches[i].Phase = phase
	}
	if len(mismatches) == 0 {
		return
	}
	var report strings.Builder
	for _, mismatch := range mismatches {
		fmt.Fprintf(&report, "%s at %s: old=%s new=%s\n", mismatch.Phase, mismatch.Path, mismatch.Old, mismatch.New)
	}
	t.Fatalf("equivalence mismatches (%d):\n%s", len(mismatches), report.String())
}

func compareEquivalent(old, new reflect.Value, path string) []equivalenceMismatch {
	if !old.IsValid() || !new.IsValid() {
		if old.IsValid() == new.IsValid() {
			return nil
		}
		return []equivalenceMismatch{{Path: path, Old: valueString(old), New: valueString(new)}}
	}
	if old.Type() != new.Type() {
		return []equivalenceMismatch{{Path: path, Old: valueString(old), New: valueString(new)}}
	}
	switch old.Kind() {
	case reflect.Interface:
		if old.IsNil() || new.IsNil() {
			if old.IsNil() == new.IsNil() {
				return nil
			}
			return []equivalenceMismatch{{Path: path, Old: valueString(old), New: valueString(new)}}
		}
		return compareEquivalent(old.Elem(), new.Elem(), path)
	case reflect.Pointer:
		if old.IsNil() || new.IsNil() {
			if old.IsNil() == new.IsNil() {
				return nil
			}
			return []equivalenceMismatch{{Path: path, Old: valueString(old), New: valueString(new)}}
		}
		return compareEquivalent(old.Elem(), new.Elem(), path)
	case reflect.Struct:
		var out []equivalenceMismatch
		for i := 0; i < old.NumField(); i++ {
			if old.Type().Field(i).PkgPath != "" { // unexported implementation state
				continue
			}
			out = append(out, compareEquivalent(old.Field(i), new.Field(i), path+"."+old.Type().Field(i).Name)...)
		}
		return out
	case reflect.Slice, reflect.Array:
		if old.Len() != new.Len() {
			return []equivalenceMismatch{{Path: path + ".len", Old: fmt.Sprint(old.Len()), New: fmt.Sprint(new.Len())}}
		}
		var out []equivalenceMismatch
		for i := 0; i < old.Len(); i++ {
			out = append(out, compareEquivalent(old.Index(i), new.Index(i), fmt.Sprintf("%s[%d]", path, i))...)
		}
		return out
	default:
		if reflect.DeepEqual(old.Interface(), new.Interface()) {
			return nil
		}
		return []equivalenceMismatch{{Path: path, Old: valueString(old), New: valueString(new)}}
	}
}

func valueString(v reflect.Value) string {
	if !v.IsValid() {
		return "<invalid>"
	}
	if (v.Kind() == reflect.Interface || v.Kind() == reflect.Pointer) && v.IsNil() {
		return "<nil>"
	}
	return fmt.Sprintf("%v", v.Interface())
}

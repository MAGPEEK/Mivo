package spkmeta

import (
	"errors"
	"testing"
)

// The public renderers use fixed names; this guards the grammar's name part against a future
// field with a malformed name.
func TestRenderEnvRejectsNamesOutsideTheGrammar(t *testing.T) {
	t.Parallel()

	for _, name := range []string{"", "lower", "1ID", "DISPLAY-NAME", "ID ", "ÄRGER"} {
		if _, err := renderEnv([]envAssignment{{name: name, value: "x"}}); !errors.Is(err, ErrInvalidValue) {
			t.Errorf("renderEnv(%q) = %v, want ErrInvalidValue", name, err)
		}
	}

	for _, value := range []string{"it's", "a\nb", "a\r", "\xff"} {
		if _, err := renderEnv([]envAssignment{{name: "A", value: value}}); !errors.Is(err, ErrUnsafeValue) {
			t.Errorf("renderEnv(A=%q) = %v, want ErrUnsafeValue", value, err)
		}
	}

	got, err := renderEnv([]envAssignment{{name: "_A1", value: ""}, {name: "B", value: "x y"}})
	if err != nil || got != "_A1=''\nB='x y'\n" {
		t.Errorf("renderEnv() = %q, %v", got, err)
	}
}

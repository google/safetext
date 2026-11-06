// Copyright 2026 Google LLC.
//
// Licensed under the Apache License, Version 2.0 (the "License");
// you may not use this file except in compliance with the License.
// You may obtain a copy of the License at
//
//     http://www.apache.org/licenses/LICENSE-2.0
//
// Unless required by applicable law or agreed to in writing, software
// distributed under the License is distributed on an "AS IS" BASIS,
// WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
// See the License for the specific language governing permissions and
// limitations under the License.

package shsprintf_test

import (
	"errors"
	"strings"
	"testing"

	"github.com/google/safetext/shsprintf"
)

func TestSafetextShsprintf(t *testing.T) {
	type testCase struct {
		format       string
		replacements []any
		err          error
	}

	testCases := []testCase{
		// Negative cases
		// ====================================================
		{
			format: "echo %s",
			replacements: []any{
				"hello",
			},
			err: nil,
		},

		{
			format: "echo %s",
			replacements: []any{
				// '+' is allowed
				"Just+A+Random+String",
			},
			err: nil,
		},

		{
			format: "echo %s",
			replacements: []any{
				// '@' is allowed
				"like@an.email",
			},
			err: nil,
		},

		{
			format: `echo "%s"`,
			replacements: []any{
				"hello hello",
			},
			err: nil,
		},

		{
			format: `echo '%s''s '`,
			replacements: []any{
				"blas hello",
			},
			err: nil,
		},

		{
			format: `command "x-%s-y-%s"`,
			replacements: []any{
				"ar\\\"g-yz",
				"bla",
			},
			err: nil,
		},

		// Truncation
		{
			format: `command "%.4s"`,
			replacements: []any{
				"bla",
			},
			err: nil,
		},

		{
			format: `ls %s`,
			replacements: []any{
				"/tmp" + "/bla",
			},
			err: nil,
		},

		{
			format: "echo `cat %s`",
			replacements: []any{
				"hello",
			},
			err: nil,
		},

		{
			format: `#! /bin/bash
end=$((SECONDS+%d))

while [ $SECONDS -lt $end ]; do
:
done`,
			replacements: []any{
				3,
			},
			err: nil,
		},

		{
			format: `echo "$(( %s + %s ))"`,
			replacements: []any{
				"12",
				"10",
			},
			err: nil,
		},

		// String values in items in loops can change (as long as new expressions not introduced)
		{
			format: `for VARIABLE in file1 %s file3
do
    cat $VARIABLE
done`,
			replacements: []any{
				"bla",
			},
			err: nil,
		},

		// C-style loop
		{
			format: `for (( c=1; c<=%s; c++ ))
do
  shell_COMMANDS
done`,
			replacements: []any{
				"5",
			},
			err: nil,
		},

		// If statement conditions fine to change
		{
			format: `if [ %s ] ; then
command
fi`,
			replacements: []any{
				"condition",
			},
			err: nil,
		},

		// Extended if
		{
			format: `
if [[ -e %s ]]
then
  echo "File exists"
fi`,
			replacements: []any{
				"file",
			},
			err: nil,
		},

		// Positive cases
		// ====================================================
		// Command injection
		{
			format: "echo %s",
			replacements: []any{
				"`./command`",
			},
			err: shsprintf.ErrShInjection,
		},

		// Special characters
		{
			format: "echo %s",
			replacements: []any{
				// globbing chars ('?') are not allowed
				"Just?ARandomString",
			},
			err: shsprintf.ErrShInjection,
		},

		{
			format: "echo %s",
			replacements: []any{
				// globbing chars ('*') are not allowed
				"Just*ARandomString",
			},
			err: shsprintf.ErrShInjection,
		},

		{
			format: "echo %s",
			replacements: []any{
				// ! is command history
				"Just!ARandomString",
			},
			err: shsprintf.ErrShInjection,
		},

		// Multiple arguments
		{
			format: "cat %s",
			replacements: []any{
				"one two three",
			},
			err: shsprintf.ErrShInjection,
		},

		// Flags
		{
			format: "cat %s",
			replacements: []any{
				"-flag",
			},
			err: shsprintf.ErrShInjection,
		},

		// Condition with new expressions
		{
			format: `
if [[ -e %s ]]
then
  echo "File exists"
fi`,
			replacements: []any{
				"file || (1==1)",
			},
			err: shsprintf.ErrShInjection,
		},

		// Command injection 2
		{
			format: "echo %s",
			replacements: []any{
				"$(./command)",
			},
			err: shsprintf.ErrShInjection,
		},

		// Command injection 3
		{
			format: "echo %s",
			replacements: []any{
				"foobar\ncommand",
			},
			err: shsprintf.ErrShInjection,
		},

		// Command injection 4
		{
			format: "echo %s foobar",
			replacements: []any{
				";",
			},
			err: shsprintf.ErrShInjection,
		},

		// Command injection 5
		{
			format: "echo %s",
			replacements: []any{
				"foo$(./command)bar",
			},
			err: shsprintf.ErrShInjection,
		},

		// Command injection 6
		{
			format: "echo %s",
			replacements: []any{
				"foo`./command`bar",
			},
			err: shsprintf.ErrShInjection,
		},

		// Command injection 7
		{
			format: "echo %s",
			replacements: []any{
				`"foo$(./command)bar"`,
			},
			err: shsprintf.ErrShInjection,
		},

		// Command injection 7 - cross check
		{
			format: "echo %s",
			replacements: []any{
				`foo/commandbar`,
			},
			err: nil,
		},

		// Command injection 8
		{
			format: `echo "%s"`,
			replacements: []any{
				`foo$(./command)bar`,
			},
			err: shsprintf.ErrShInjection,
		},

		// Command injection 9
		{
			format: `echo '%s'`,
			replacements: []any{
				`foo' ./command 'bar`,
			},
			err: shsprintf.ErrShInjection,
		},

		// Argument injection due to glob operator in literal strings
		{
			format: `touch %s; echo %s`,
			replacements: []any{
				"./--some-param=value",
				"*value",
			},
			err: shsprintf.ErrShInjection,
		},

		// Argument injection via backslash escape: bash removes the backslash,
		// so `rm \-rf` executes as `rm -rf`.
		{
			format: "rm %s",
			replacements: []any{
				`\-rf`,
			},
			err: shsprintf.ErrShInjection,
		},

		// Argument injection via brace expansion: `rm {-rf,}` expands to `rm -rf`.
		{
			format: "rm %s",
			replacements: []any{
				"{-rf,}",
			},
			err: shsprintf.ErrShInjection,
		},

		{
			format: "rm %s",
			replacements: []any{
				`{rf,bd}`,
			},
			err: shsprintf.ErrShInjection,
		},

		// Value within a brace expansion of the template: `rm {foo,-rf}` expands
		// to `rm foo -rf`.
		{
			format: "rm {foo,%s}",
			replacements: []any{
				"-rf",
			},
			err: shsprintf.ErrShInjection,
		},

		// Brace expansion with a quoted element: `rm {"-rf",}` expands to `rm -rf`.
		{
			format: "rm %s",
			replacements: []any{
				`{"-rf",}`,
			},
			err: shsprintf.ErrShInjection,
		},

		// An escaped backslash is passed literally (`\-rf`), so no flag is injected.
		{
			format: "rm %s",
			replacements: []any{
				`\\-rf`,
			},
			err: nil,
		},

		// Backslashes not preceding a dash are fine.
		{
			format: "echo %s",
			replacements: []any{
				`a\b`,
			},
			err: nil,
		},

		// Braces without a comma or sequence are not expanded.
		{
			format: "echo %s",
			replacements: []any{
				"{foo}",
			},
			err: nil,
		},

		// Braces are not expanded within double quotes.
		{
			format: `echo "%s"`,
			replacements: []any{
				"{-rf,}",
			},
			err: nil,
		},

		// Brace expansions in the template itself are fine.
		{
			format: "cp %s{,.bak}",
			replacements: []any{
				"file",
			},
			err: nil,
		},

		// Sequence brace expansions in the template itself are fine.
		{
			format: "echo {1..3}%s",
			replacements: []any{
				"a",
			},
			err: nil,
		},

		// `rm x{foo,-rf}` expands to `rm xfoo x-rf`, so no flag is injected.
		{
			format: "rm x{foo,%s}",
			replacements: []any{
				"-rf",
			},
			err: nil,
		},

		// `rm {,x}-rf` expands to `rm -rf x-rf`: an empty brace element exposes
		// the value at the start of an argument.
		{
			format: "rm {,x}%s",
			replacements: []any{
				"-rf",
			},
			err: shsprintf.ErrShInjection,
		},

		// A parameter expansion may be empty, so the value may start the argument.
		{
			format: `rm "$y"%s`,
			replacements: []any{
				"-rf",
			},
			err: shsprintf.ErrShInjection,
		},

		// A backslash within double quotes is preserved, so `rm "\-rf"` passes `\-rf`.
		{
			format: `rm "%s"`,
			replacements: []any{
				`\-rf`,
			},
			err: nil,
		},

		// ANSI-C quoting: `$'\x2drf'` is `-rf`.
		{
			format: `rm $'%s'`,
			replacements: []any{
				`\x2drf`,
			},
			err: shsprintf.ErrShInjection,
		},

		// Brace expansion injected within a command substitution.
		{
			format: "echo $(rm %s)",
			replacements: []any{
				"{-rf,}",
			},
			err: shsprintf.ErrShInjection,
		},

		// Variable assignments do not undergo brace expansion in bash.
		{
			format: "x=%s",
			replacements: []any{
				"{-rf,}",
			},
			err: nil,
		},

		// Unindexed array elements undergo brace expansion in bash.
		{
			format: "x=(%s)",
			replacements: []any{
				"{a,b}",
			},
			err: shsprintf.ErrShInjection,
		},

		// Indexed array elements do not undergo brace expansion in bash.
		{
			format: "x=([0]=%s)",
			replacements: []any{
				"{-rf,}",
			},
			err: nil,
		},

		// Items in a for loop undergo brace expansion in bash.
		{
			format: "for x in %s; do echo $x; done",
			replacements: []any{
				"{a,b}",
			},
			err: shsprintf.ErrShInjection,
		},

		// Arguments to export/declare undergo brace expansion in bash.
		{
			format: "export x=%s",
			replacements: []any{
				"{a,b}",
			},
			err: shsprintf.ErrShInjection,
		},

		// Ambiguous redirects through brace expansion is blocked.
		{
			format: "echo foo > %s",
			replacements: []any{
				"{a,b}",
			},
			err: shsprintf.ErrShInjection,
		},

		// Turning a list brace expansion into a sequence brace expansion.
		{
			format: "echo {1..%s,b}x",
			replacements: []any{
				"2}",
			},
			err: shsprintf.ErrShInjection,
		},

		// Here-document bodies do not undergo brace expansion in bash.
		{
			format: "cat << EOF\n%s\nEOF\n",
			replacements: []any{
				"{\n  \"live-restore\": true,\n  \"log-opts\": {\n    \"max-size\": \"1g\"\n  }\n}",
			},
			err: nil,
		},

		// Indented here-document bodies do not undergo brace expansion in bash.
		{
			format: "cat <<- EOF\n%s\nEOF\n",
			replacements: []any{
				"{-rf,}",
			},
			err: nil,
		},

		// Here-strings do not undergo brace expansion in bash.
		{
			format: "cat <<< %s",
			replacements: []any{
				"{-rf,}",
			},
			err: nil,
		},

		// Brace expansion injected within a command substitution in a here-document.
		{
			format: "cat << EOF\n$(rm %s)\nEOF\n",
			replacements: []any{
				"{-rf,}",
			},
			err: shsprintf.ErrShInjection,
		},

		// ANSI-C style
		{
			format: `command %s`,
			replacements: []any{
				`$'\u002d\u002d'flag=value`,
			},
			err: shsprintf.ErrShInjection,
		},
	}

	for _, tc := range testCases {
		t.Run(tc.format, func(t *testing.T) {
			result, err := shsprintf.Sprintf(tc.format, tc.replacements...)
			if err != tc.err {
				t.Errorf("Got %v, expected %v", err, tc.err)

				if err == nil {
					t.Logf("template execution result was %s", result)
				}
			}
		})
	}
}

// The reason we don't need the mutation pass for shell scripting, like is done in the YAML code, is because the parser operates on the AST-level and can parse the exact number of quotes / comments, detecting cross-argument injections where the structure would otherwise match, as below:
func TestSafetextShsprintfComment(t *testing.T) {
	controlled := "x; commandB injection #"
	notControlled := "notControlled"

	result, err := shsprintf.Sprintf("commandA %s; commandB %s # Comment", controlled, notControlled)

	if err != shsprintf.ErrShInjection {
		t.Errorf("Got %v, expected %v", err, shsprintf.ErrShInjection)

		if err == nil {
			t.Logf("template execution result was %s", result)
		}
	}
}

func TestSafetextShsprintfQuote(t *testing.T) {
	controlled := "x; commandB injection; commandC \""
	notControlled := "notControlled"
	c := "\\"

	result, err := shsprintf.Sprintf("commandA %s; commandB %s; commandC \"%s\"", controlled, notControlled, c)

	if err != shsprintf.ErrShInjection {
		t.Errorf("Got %v, expected %v\n", err, shsprintf.ErrShInjection)

		if err == nil {
			t.Logf("template execution result was %s", result)
		}
	}
}

// Demonstration of how to paste multiple arguments
func TestSafetextShsprintfMultipleArguments(t *testing.T) {
	files := []any{"file1", "file2", "file3"}

	_, err := shsprintf.Sprintf("cat"+strings.Repeat(" %s", len(files)), files...)

	if err != nil {
		t.Errorf("Unexpected Sprintf error: %v", err)
	}
}

func generateExportVariables(env map[string]string) (string, error) {
	script := new(strings.Builder)

	// Note: iteration order undefined
	for key, value := range env {
		result, err := shsprintf.Sprintf("export %s=%s\n", key, value)

		if err != nil {
			return "", err
		}

		script.WriteString(result)
	}

	return script.String(), nil
}

// Demonstration of using map of environment variables
func TestSafetextShsprintfMapsNegative(t *testing.T) {
	_, err := generateExportVariables(map[string]string{"one": "a", "two": "b", "three": "c"})

	if err != nil {
		t.Errorf("Unexpected Sprintf error: %v", err)
	}
}

func TestSafetextShsprintfMapsPositive(t *testing.T) {
	_, err := generateExportVariables(map[string]string{"one": "a", "two": "b", "three": "c four=d"})

	if err != shsprintf.ErrShInjection {
		t.Errorf("Got %v, expected %v", err, shsprintf.ErrShInjection)
	}
}

func TestSafetextShsprintfMultilineEof(t *testing.T) {
	path := "bla"
	content := `test
another line`

	_, err := shsprintf.Sprintf(`cat > %s/settings.xml << 'EOF'
%s
EOF`, path, content)

	if err != nil {
		t.Errorf("Unexpected Sprintf error: %v", err)
	}
}

func TestSafetextShsprintfEscape(t *testing.T) {
	var arg strings.Builder
	arg.WriteString("bla")

	for i := range 256 {
		arg.WriteString(string(rune(i)))
	}

	_, err := shsprintf.Sprintf(`cmd --arg=%s`, shsprintf.EscapeDefaultContext(arg.String()))

	if err != nil {
		t.Errorf("Unexpected Sprintf error: %v", err)
	}
}

func TestSafetextShsprintfInvalidTemplate(t *testing.T) {
	_, err := shsprintf.Sprintf("echo 'unclosed %s", "hello")
	if err == nil {
		t.Fatalf("Got nil, expected error for invalid shell template")
	}
	if !errors.Is(err, shsprintf.ErrInvalidShTemplate) {
		t.Errorf("Got error %v, expected it to wrap ErrInvalidShTemplate", err)
	}
	if err.Error() == shsprintf.ErrInvalidShTemplate.Error() {
		t.Errorf("Got error %v, expected it to contain underlying parser error details", err)
	}
}

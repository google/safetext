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

package shtemplate_test

import (
	"bytes"
	"testing"

	template "github.com/google/safetext/shtemplate"
)

func TestSafetextShtemplate(t *testing.T) {
	type testCase struct {
		tmplText     string
		replacements map[any]any
		err          error
	}

	testCases := []testCase{
		// Negative cases
		// ====================================================
		{
			tmplText: "echo {{ .addressee }}",
			replacements: map[any]any{
				"addressee": "hello",
			},
			err: nil,
		},

		{
			tmplText: `echo "{{ .addressee }}"`,
			replacements: map[any]any{
				"addressee": "hello hello",
			},
			err: nil,
		},

		{
			tmplText: `ls {{ range .Paths }}{{.}} {{end}}`,
			replacements: map[any]any{
				"Paths": []string{"/tmp", "/bla"},
			},
			err: nil,
		},

		{
			tmplText: "echo `cat {{ .file }}`",
			replacements: map[any]any{
				"file": "hello",
			},
			err: nil,
		},

		{
			tmplText: `#! /bin/bash
end=$((SECONDS+{{.wait}}))

while [ $SECONDS -lt $end ]; do
:
done`,
			replacements: map[any]any{
				"wait": 3,
			},
			err: nil,
		},

		{
			tmplText: `echo "$(( {{.a}} + {{.b}} ))"`,
			replacements: map[any]any{
				"a": "12",
				"b": "10",
			},
			err: nil,
		},

		// String values in items in loops can change (as long as new expressions not introduced)
		{
			tmplText: `for VARIABLE in file1 {{.a}} file3
do
    cat $VARIABLE
done`,
			replacements: map[any]any{
				"a": "bla",
			},
			err: nil,
		},

		// C-style loop
		{
			tmplText: `for (( c=1; c<={{.a}}; c++ ))
do
  shell_COMMANDS
done`,
			replacements: map[any]any{
				"a": "5",
			},
			err: nil,
		},

		// If statement conditions fine to change
		{
			tmplText: `if [ {{.c}} ] ; then
command
fi`,
			replacements: map[any]any{
				"c": "condition",
			},
			err: nil,
		},

		// Extended if
		{
			tmplText: `
if [[ -e {{.x}} ]]
then
  echo "File exists"
fi`,
			replacements: map[any]any{
				"x": "file",
			},
			err: nil,
		},

		// Explicitly annotated command
		{
			tmplText: "echo $({{ (StructuralData .addressee) }})",
			replacements: map[any]any{
				"addressee": "./command",
			},
			err: nil,
		},

		// Explicitly annotated filename
		{
			tmplText: "bla > {{ StructuralData .addressee }}",
			replacements: map[any]any{
				"addressee": "filename",
			},
			err: nil,
		},

		// Explicitly annotated flag
		{
			tmplText: "bla {{ AllowFlags .addressee }}",
			replacements: map[any]any{
				"addressee": "-flag",
			},
			err: nil,
		},

		// Positive cases
		// ====================================================
		// Unannotated command
		{
			tmplText: "echo $({{ .addressee }})",
			replacements: map[any]any{
				"addressee": "./command",
			},
			err: template.ErrShInjection,
		},

		// Command injection
		{
			tmplText: "echo {{ .addressee }}",
			replacements: map[any]any{
				"addressee": "`./command`",
			},
			err: template.ErrShInjection,
		},

		// Multiple arguments
		{
			tmplText: "cat {{ .addressee }}",
			replacements: map[any]any{
				"addressee": "one two three",
			},
			err: template.ErrShInjection,
		},

		// Unannotated flags
		{
			tmplText: "cat {{ .addressee }}",
			replacements: map[any]any{
				"addressee": "-flag",
			},
			err: template.ErrShInjection,
		},

		// Unannotated filenames
		{
			tmplText: "bla > {{ .addressee }}",
			replacements: map[any]any{
				"addressee": "filename",
			},
			err: template.ErrShInjection,
		},

		// Filename with syntax manipulation
		{
			tmplText: "bla {{ .addressee }}>out",
			replacements: map[any]any{
				"addressee": "x>evil#",
			},
			err: template.ErrShInjection,
		},

		// Condition with new expressions
		{
			tmplText: `
if [[ -e {{.x}} ]]
then
  echo "File exists"
fi`,
			replacements: map[any]any{
				"x": "file || (1==1)",
			},
			err: template.ErrShInjection,
		},

		// Command injection 2
		{
			tmplText: "echo {{ .addressee }}",
			replacements: map[any]any{
				"addressee": "$(./command)",
			},
			err: template.ErrShInjection,
		},

		// Command injection 3
		{
			tmplText: "echo {{ .addressee }}",
			replacements: map[any]any{
				"addressee": "foobar\ncommand",
			},
			err: template.ErrShInjection,
		},

		// Command injection 4
		{
			tmplText: "echo {{ .addressee }} foobar",
			replacements: map[any]any{
				"addressee": ";",
			},
			err: template.ErrShInjection,
		},

		// Command injection 5
		{
			tmplText: "echo {{ .addressee }}",
			replacements: map[any]any{
				"addressee": "foo$(./command)bar",
			},
			err: template.ErrShInjection,
		},

		// Command injection 6
		{
			tmplText: "echo {{ .addressee }}",
			replacements: map[any]any{
				"addressee": "foo`./command`bar",
			},
			err: template.ErrShInjection,
		},

		// Command injection 7
		{
			tmplText: "echo {{ .addressee }}",
			replacements: map[any]any{
				"addressee": `"foo$(./command)bar"`,
			},
			err: template.ErrShInjection,
		},

		// Command injection 7 - cross check
		{
			tmplText: "echo {{ .addressee }}",
			replacements: map[any]any{
				"addressee": `foo/commandbar`,
			},
			err: nil,
		},

		// Command injection 8
		{
			tmplText: `echo "{{ .addressee }}"`,
			replacements: map[any]any{
				"addressee": `foo$(./command)bar`,
			},
			err: template.ErrShInjection,
		},

		// Command injection 9
		{
			tmplText: `echo '{{ .addressee }}'`,
			replacements: map[any]any{
				"addressee": `foo' ./command 'bar`,
			},
			err: template.ErrShInjection,
		},

		// Argument injection due to glob operator in literal strings
		{
			tmplText: `touch {{.a}}; echo {{.b}}`,
			replacements: map[any]any{
				"a": "./--some-param=value",
				"b": "*value",
			},
			err: template.ErrShInjection,
		},

		// Command injection via plain field access (control for the ChainNode case below)
		{
			tmplText: "echo {{ .addressee }}",
			replacements: map[any]any{
				"addressee": "a; rm -rf /",
			},
			err: template.ErrShInjection,
		},

		// Command injection via ChainNode field access must not bypass detection
		{
			tmplText: "echo {{ (.).addressee }}",
			replacements: map[any]any{
				"addressee": "a; rm -rf /",
			},
			err: template.ErrShInjection,
		},

		// Redirect injection
		{
			tmplText: "echo {{.X}}",
			replacements: map[any]any{
				"X": "arg > /tmp/pwn",
			},
			err: template.ErrShInjection,
		},

		// Extra statement injection inside a nested block
		{
			tmplText: "if true; then\necho {{.X}}\nfi",
			replacements: map[any]any{
				"X": "a\nrm -rf /tmp/victim",
			},
			err: template.ErrShInjection,
		},

		// Argument injection via backslash escape: bash removes the backslash,
		// so `rm \-rf` executes as `rm -rf`.
		{
			tmplText: "rm {{.X}}",
			replacements: map[any]any{
				"X": `\-rf`,
			},
			err: template.ErrShInjection,
		},

		// Argument injection via brace expansion: `rm {-rf,}` expands to `rm -rf`.
		{
			tmplText: "rm {{.X}}",
			replacements: map[any]any{
				"X": "{-rf,}",
			},
			err: template.ErrShInjection,
		},

		{
			tmplText: "rm {{.X}}",
			replacements: map[any]any{
				"X": `{rf,bd}`,
			},
			err: template.ErrShInjection,
		},

		// Value within a brace expansion of the template: `rm {foo,-rf}` expands
		// to `rm foo -rf`.
		{
			tmplText: "rm {foo,{{.X}}}",
			replacements: map[any]any{
				"X": "-rf",
			},
			err: template.ErrShInjection,
		},

		// Brace expansion with a quoted element: `rm {"-rf",}` expands to `rm -rf`.
		{
			tmplText: "rm {{.X}}",
			replacements: map[any]any{
				"X": `{"-rf",}`,
			},
			err: template.ErrShInjection,
		},

		// An escaped backslash is passed literally (`\-rf`), so no flag is injected.
		{
			tmplText: "rm {{.X}}",
			replacements: map[any]any{
				"X": `\\-rf`,
			},
			err: nil,
		},

		// Backslashes not preceding a dash are fine.
		{
			tmplText: "echo {{.X}}",
			replacements: map[any]any{
				"X": `a\b`,
			},
			err: nil,
		},

		// Braces without a comma or sequence are not expanded.
		{
			tmplText: "echo {{.X}}",
			replacements: map[any]any{
				"X": "{foo}",
			},
			err: nil,
		},

		// Braces are not expanded within double quotes.
		{
			tmplText: `echo "{{.X}}"`,
			replacements: map[any]any{
				"X": "{-rf,}",
			},
			err: nil,
		},

		// Brace expansions in the template itself are fine.
		{
			tmplText: "cp {{.X}}{,.bak}",
			replacements: map[any]any{
				"X": "file",
			},
			err: nil,
		},

		// Sequence brace expansions in the template itself are fine.
		{
			tmplText: "echo {1..3}{{.X}}",
			replacements: map[any]any{
				"X": "a",
			},
			err: nil,
		},

		// `rm x{foo,-rf}` expands to `rm xfoo x-rf`, so no flag is injected.
		{
			tmplText: "rm x{foo,{{.X}}}",
			replacements: map[any]any{
				"X": "-rf",
			},
			err: nil,
		},

		// `rm {,x}-rf` expands to `rm -rf x-rf`: an empty brace element exposes
		// the value at the start of an argument.
		{
			tmplText: "rm {,x}{{.X}}",
			replacements: map[any]any{
				"X": "-rf",
			},
			err: template.ErrShInjection,
		},

		// A parameter expansion may be empty, so the value may start the argument.
		{
			tmplText: `rm "$y"{{.X}}`,
			replacements: map[any]any{
				"X": "-rf",
			},
			err: template.ErrShInjection,
		},

		// A backslash within double quotes is preserved, so `rm "\-rf"` passes `\-rf`.
		{
			tmplText: `rm "{{.X}}"`,
			replacements: map[any]any{
				"X": `\-rf`,
			},
			err: nil,
		},

		// ANSI-C quoting: `$'\x2drf'` is `-rf`.
		{
			tmplText: `rm $'{{.X}}'`,
			replacements: map[any]any{
				"X": `\x2drf`,
			},
			err: template.ErrShInjection,
		},

		// Brace expansion injected within a command substitution.
		{
			tmplText: "echo $(rm {{.X}})",
			replacements: map[any]any{
				"X": "{-rf,}",
			},
			err: template.ErrShInjection,
		},

		// Variable assignments do not undergo brace expansion in bash.
		{
			tmplText: "x={{.X}}",
			replacements: map[any]any{
				"X": "{-rf,}",
			},
			err: nil,
		},

		// Unindexed array elements undergo brace expansion in bash.
		{
			tmplText: "x=({{.X}})",
			replacements: map[any]any{
				"X": "{a,b}",
			},
			err: template.ErrShInjection,
		},

		// Indexed array elements do not undergo brace expansion in bash.
		{
			tmplText: "x=([0]={{.X}})",
			replacements: map[any]any{
				"X": "{-rf,}",
			},
			err: nil,
		},

		// Items in a for loop undergo brace expansion in bash.
		{
			tmplText: "for x in {{.X}}; do echo $x; done",
			replacements: map[any]any{
				"X": "{a,b}",
			},
			err: template.ErrShInjection,
		},

		// Arguments to export/declare undergo brace expansion in bash.
		{
			tmplText: "export x={{.X}}",
			replacements: map[any]any{
				"X": "{a,b}",
			},
			err: template.ErrShInjection,
		},

		// Ambiguous redirects through brace expansion is blocked.
		{
			tmplText: "echo foo > {{.X}}",
			replacements: map[any]any{
				"X": "{a,b}",
			},
			err: template.ErrShInjection,
		},

		// Turning a list brace expansion into a sequence brace expansion.
		{
			tmplText: "echo {1..{{.X}},b}x",
			replacements: map[any]any{
				"X": "2}",
			},
			err: template.ErrShInjection,
		},

		// Here-document bodies do not undergo brace expansion in bash.
		{
			tmplText: "cat << EOF\n{\n  \"live-restore\": true,\n  \"hostname\": \"$(echo \"{{.X}}\")\"\n}\nEOF\n",
			replacements: map[any]any{
				"X": "{-rf,}",
			},
			err: nil,
		},

		// Indented here-document bodies do not undergo brace expansion in bash.
		{
			tmplText: "cat <<- EOF\n{\n  \"live-restore\": true,\n  \"hostname\": \"$(echo \"{{.X}}\")\"\n}\nEOF\n",
			replacements: map[any]any{
				"X": "{-rf,}",
			},
			err: nil,
		},

		// Here-strings do not undergo brace expansion in bash.
		{
			tmplText: "cat <<< {a,b}$(echo \"{{.X}}\")",
			replacements: map[any]any{
				"X": "{-rf,}",
			},
			err: nil,
		},

		// Brace expansion injected within a command substitution in a here-document.
		{
			tmplText: "cat << EOF\n$(rm {{.X}})\nEOF\n",
			replacements: map[any]any{
				"X": "{-rf,}",
			},
			err: template.ErrShInjection,
		},
	}

	for _, tc := range testCases {
		t.Run(tc.tmplText, func(t *testing.T) {
			tmpl, _ := template.New("test").Parse(tc.tmplText)
			var buf bytes.Buffer
			err := tmpl.Execute(&buf, tc.replacements)

			if err != tc.err {
				t.Errorf("Expected %v, got %v\n", tc.err, err)

				if err == nil {
					t.Logf("template execution result was %s\n", buf.String())
				}
			}
		})
	}
}

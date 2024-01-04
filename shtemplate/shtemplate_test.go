// Copyright 2023 Google LLC.
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
	template "github.com/google/safetext/shtemplate"

	"bytes"
	"testing"
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
			tmplText: "echo $(({{ (StructuralData .addressee) }}))",
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
			tmplText: "echo $(({{ .addressee }}))",
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

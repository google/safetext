// Copyright 2024 Google LLC.
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

package yamltemplate_test

import (
	"bytes"
	"embed"
	"fmt"
	"strconv"
	"strings"
	"testing"

	template "github.com/google/safetext/yamltemplate"
)

//go:embed list.yaml.tmpl
var eListYamlTmpl embed.FS

//go:embed nested-template.yaml.tmpl
var nestedTemplateYamlTmpl embed.FS

//go:embed nested-template-inner.yaml.tmpl
var nestedTemplateInnerYamlTmpl embed.FS

func TestSafetextYamltemplate(t *testing.T) {
	type testCase struct {
		tmplText     string
		replacements map[any]any
		err          bool
	}

	testCases := []testCase{
		// Negative cases
		{
			tmplText: "{ hello: \"{{ .addressee | js }}\" }",
			replacements: map[any]any{
				"addressee": "world\", inject: \"oops",
			},
			err: false,
		},

		{
			tmplText: `
---
- stream: one,
- hello: {{ .addressee }},
---
- stream: two,
- hello: {{ .addressee }},
`,
			replacements: nil,
			err:          false,
		},

		{
			tmplText: `
data:
  HTTPS_PROXY: {{.p1}}
  NO_PROXY: {{.p2}}
`,
			replacements: map[any]any{
				"p1": "1",
				"p2": "localhost, 127.0.0.1",
			},
			err: false,
		},

		{
			tmplText: `
data:
  HTTPS_PROXY: {{.p1}}
  NO_PROXY: {{.p2}}
`,
			replacements: map[any]any{
				"p1": "",
				"p2": "localhost, 127.0.0.1",
			},
			err: false,
		},

		{
			tmplText: "{ {{ if not .hide }}hello: {{ .addressee }}{{end}} }",
			replacements: map[any]any{
				"addressee": "world",
				"hide":      false,
			},
			err: false,
		},

		{
			tmplText: "{ {{ if eq .addressee \"world\" }}hello: {{ .addressee }}{{end}} }",
			replacements: map[any]any{
				"addressee": "world",
			},
			err: false,
		},

		{
			tmplText: `{ list: "{{ range .entries }}{{.}}{{ end }}" }`,
			replacements: map[any]any{
				"entries": []string{"(special characters to not trigger fast path {})", "two", "three"},
			},
			err: false,
		},

		{
			tmplText: `
list:
{{with .some_field}}
{{if eq . "x"}}
- {{.}}
{{end}}
{{end}}
`,
			replacements: map[any]any{
				"some_field": "x",
				"slow":       "{}",
			},
			err: false,
		},

		{
			tmplText: "{ test: bla }",
			replacements: map[any]any{
				0: "(special characters to not trigger fast path {})",
			},
			err: false,
		},

		// Verify that unused replacements in nested yaml don't cause templates to fail
		{
			tmplText: `hello:
- to: {{ .addressee }}
  next:
  - first: test
`,
			replacements: map[any]any{
				"addressee": "world",
				"unused":    "some-thing",
			},
			err: false,
		},

		// Verify that valid strings with non-standard characters work in nested yaml
		{
			tmplText: `hello:
- to: {{ .addressee }}
  next:
  - first: test
`,
			replacements: map[any]any{
				"addressee": "whole-world",
			},
			err: false,
		},

		// Verify that internal YAML parser rejects duplicate keys
		{
			tmplText: "{ hello: {{ .addressee }}, hello: multiple }",
			replacements: map[any]any{
				"addressee": "world",
			},
			err: true,
		},

		// Verify that internal YAML parsers rejects map keys
		{
			tmplText: "{ {}: {{ .addressee }} }",
			replacements: map[any]any{
				"addressee": "world",
			},
			err: true,
		},

		// Verify that internal YAML parsers rejects slice keys
		{
			tmplText: "{ [1, 2, 3]: {{ .addressee }} }",
			replacements: map[any]any{
				"addressee": "world",
			},
			err: true,
		},

		// Verify that YAML parses still accepts "non-strict" YAML (whilst rejecting duplicate keys)
		{
			tmplText: "a: {{ .addressee }}",
			replacements: map[any]any{
				"addressee": "world",
			},
			err: false,
		},

		// nil type
		{
			tmplText: `{ a: {{.a}}, b: '{{.b}}' }`,
			replacements: map[any]any{
				"a": nil,
				"b": "{}",
			},
			err: false,
		},

		// Positive cases
		{
			tmplText: "{ hello: \"{{ .addressee }}\" }",
			replacements: map[any]any{
				"addressee": "world\", hello: \"oops_p",
			},
			err: true,
		},

		{
			tmplText: "{ hello: \"{{ .addressee }}\", parent: [ 1, {{ .s }}, 3 ] }",
			replacements: map[any]any{
				"addressee": "world",
				"s":         "2, 4",
			},
			err: true,
		},

		{
			tmplText: "{ hello: \"{{ .addressee }}\", parent: [ 1, { a: {{ .s }} }, 3 ] }",
			replacements: map[any]any{
				"addressee": "world",
				"s":         "2 , b : b",
			},
			err: true,
		},

		{
			tmplText: "{ hello: {{ .addressee }} }",
			replacements: map[any]any{
				"addressee": "{}",
			},
			err: true,
		},

		{
			tmplText: "{ {{ if eq .caddressee \"world\" }}hello: {{ .addressee }}{{end}} }",
			replacements: map[any]any{
				"caddressee": "world",
				"addressee":  "world, inject: true",
			},
			err: true,
		},

		{
			tmplText: `
---
- stream: one
- hello: a
---
- stream: two
- hello: {{ .addressee }}
`,
			replacements: map[any]any{"addressee": "world\n- inject"},
			err:          true,
		},

		// Accessing anchors should count as injected YAML syntax
		{
			tmplText: `{ secret: &secret_label 'test', disclosed: {{ .controlled }}  }`,
			replacements: map[any]any{
				"controlled": "*secret_label",
			},
			err: true,
		},
	}

	for _, tc := range testCases {
		t.Run(tc.tmplText, func(t *testing.T) {
			tmpl := template.Must(template.New("test").Parse(tc.tmplText))
			var buf bytes.Buffer
			err := tmpl.Execute(&buf, tc.replacements)

			if (err != nil) != tc.err {
				t.Errorf("tmpl.Execute: Expected %v, got %v\n", tc.err, err)
			}
		})
	}
}

// Check func maps still work
func sanitize(input any) string {
	return fmt.Sprintf("%q", input)
}

func TestSafetextYamltemplateNegativeFuncMap(t *testing.T) {
	var funcMap = map[string]any{
		"sanitize": sanitize,
	}

	tmpl := template.Must(template.New("test").Funcs(template.FuncMap(funcMap)).Parse(
		"{ a: {{ .a | sanitize }}, b: {{ .b | sanitize }} }",
	))

	replacements := map[string]any{
		"a": "world\", inject: \"oops",
		"b": "world, inject: oops",
	}

	var buf bytes.Buffer
	err := tmpl.Execute(&buf, replacements)
	if err != nil {
		t.Errorf("tmpl.Execute() error = %v", err)
	}
}

// Check structs, instead of maps
func TestSafetextYamltemplateNegativeStruct(t *testing.T) {
	tmpl := template.Must(template.New("test").Parse(
		"{ name: {{ .Name }}, age: {{ .Age }} }",
	))

	type person struct {
		Name string
		Age  int
	}

	var buf bytes.Buffer
	err := tmpl.Execute(&buf, person{Name: "bla", Age: 42})
	if err != nil {
		t.Errorf("tmpl.Execute() error = %v", err)
	}
}

func TestSafetextYamltemplatePositiveStruct(t *testing.T) {
	tmpl := template.Must(template.New("test").Parse(
		"{ name: {{ .Name }}, age: {{ .Age }} }",
	))

	type person struct {
		Name string
		Age  int
	}

	var buf bytes.Buffer
	err := tmpl.Execute(&buf, person{Name: "bla, age: 31", Age: 42})
	if err != template.ErrYAMLInjection {
		t.Errorf("Failed to detect YAML injection (%v)!", err)
	}
}

// Root node being a list instead of a map
func TestSafetextYamltemplateNegativeRootList(t *testing.T) {
	tmpl := template.Must(template.New("test").Parse(`
- one: a
- one: b
`))

	replacements := map[string]any{
		"some_field":    "x",
		"use_slow_path": "{}",
	}

	var buf bytes.Buffer
	err := tmpl.Execute(&buf, replacements)
	if err != nil {
		t.Errorf("tmpl.Execute() error = %v", err)
	}
}

// Check indirect types are followed
func TestSafetextYamltemplatePositiveIndirection(t *testing.T) {
	tmpl := template.Must(template.New("test").Parse(
		"{ name: {{ .Name }}, age: {{ .Age }} }",
	))

	type person struct {
		Name **string
		Age  int
	}

	n := "bla, age 31"
	nAddr := &n

	var buf bytes.Buffer
	err := tmpl.Execute(&buf, person{Name: &nAddr, Age: 42})
	if err != template.ErrYAMLInjection {
		t.Errorf("Failed to detect YAML injection (%v)!", err)
	}
}

// Check parsing files works
func TestSafetextYamltemplateFiles(t *testing.T) {
	tmpl, err := template.ParseFS(eListYamlTmpl, "list.yaml.tmpl")
	if err != nil {
		t.Fatalf("template.ParseFS() failed: %v", err)
	}

	replacements := map[string]any{
		"some_field":    "x",
		"use_slow_path": "{}",
	}

	var buf bytes.Buffer
	if err := tmpl.Execute(&buf, replacements); err != nil {
		t.Errorf("tmpl.Execute() error = %v", err)
	}
}

// Check methods work
type A struct {
}

func (A) GetName(n int) string { return "n is " + strconv.Itoa(n) }

func TestSafetextYamltemplateMethod(t *testing.T) {
	tmpl := template.Must(template.New("test").Parse(`- {{ (.a.GetName 0x41) | js }}`))

	replacements := map[string]any{
		"a":             A{},
		"use_slow_path": "{}",
	}

	var buf bytes.Buffer
	err := tmpl.Execute(&buf, replacements)
	if err != nil {
		t.Errorf("tmpl.Execute() error = %v", err)
	}

	if buf.String() != "- n is 65" {
		t.Errorf("Got %v, want %v\n", buf.String(), "- n is 65")
	}
}

func TestSafetextYamltemplateOptOut(t *testing.T) {
	tmpl := template.Must(template.New("test").Parse("{ Person-{{ (StructuralData .Name) }}: {{ .Age }} }"))

	type person struct {
		Name string
		Age  int
		Slow string
	}

	var buf bytes.Buffer
	err := tmpl.Execute(&buf, person{Name: "bla", Age: 42, Slow: "{}"})
	if err != nil {
		t.Errorf("tmpl.Execute() error = %v", err)
	}

	if buf.String() != "{ Person-bla: 42 }" {
		t.Errorf("Got %v, want { Person-bla: 42 }", buf.String())
	}
}

func TestCustomTypeWithStringBaseYamltemplatePositiveStruct(t *testing.T) {
	yamlTemplate := `
name: {{ .Name }}
type: {{ .PDType }}
`
	tmpl := template.Must(template.New("test").Parse(yamlTemplate))

	type PersistentDiskType string

	type StorageClassSpec struct {
		Name   string
		PDType PersistentDiskType
	}

	var buf bytes.Buffer
	err := tmpl.Execute(&buf, StorageClassSpec{Name: "ssd", PDType: "pd-ssd"})
	if err != nil {
		t.Errorf("tmpl.Execute() error = %v", err)
	}
}

// Demonstration of manually applying injection detection to the result of a function call
func readFile(path string) string {
	// (Read potentially untrusted file)

	switch path {
	case "untrusted.txt":
		return ", injection: true"
	case "safe.txt":
		return "safe"
	default:
		panic("Unknown file path!")
	}
}

func TestSafetextYamltemplateManualAnnotation(t *testing.T) {
	var funcMap = map[string]any{
		"readFile": readFile,
	}

	tmpl := template.Must(template.New("test").Funcs(template.FuncMap(funcMap)).Parse(
		"{ name: {{ readFile (StructuralData .path) | ApplyInjectionDetection }} }",
	))

	type testCase struct {
		name         string
		replacements map[string]string
		err          bool
	}

	testCases := []testCase{
		{
			name:         "negative",
			replacements: map[string]string{"path": "safe.txt"},
			err:          false,
		},
		{
			name:         "positive",
			replacements: map[string]string{"path": "untrusted.txt"},
			err:          true,
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			var buf bytes.Buffer
			err := tmpl.Execute(&buf, tc.replacements)

			if (err != nil) != tc.err {
				t.Errorf("tmpl.Execute: Expected %v, got %v\n", tc.err, err)
			}
		})
	}
}

func indent(str string, level int) string {
	pad := "\n" + strings.Repeat(" ", level)
	return strings.Replace(str, "\n", pad, -1)
}

var innerData string

func executeTemplate(tmpl *template.Template, tmplData any, indentLevel int) (string, error) {
	sb := new(strings.Builder)
	if err := tmpl.Execute(sb, map[string]any{
		"Data":  tmplData,
		"Inner": innerData,
	}); err != nil {
		return "", fmt.Errorf("failed to execute %s template: %v", tmpl.Name(), err)
	}
	return indent(sb.String(), indentLevel), nil
}

type DaemonSetConfiguration struct {
	// Name is the name of the DaemonSet
	Name string
}

func (d *DaemonSetConfiguration) GenerateLabels(indent int) (string, error) {
	labelsTmpl, err := template.ParseFS(nestedTemplateInnerYamlTmpl, "nested-template-inner.yaml.tmpl")
	if err != nil {
		return "", fmt.Errorf("template.ParseFS() failed: %v", err)
	}
	labelsMap := map[string]string{
		"one":   "foo",
		"two":   "bar",
		"three": "baz",
		"four":  "qux",
	}

	labels, err := executeTemplate(labelsTmpl, labelsMap, indent)
	if err != nil {
		return "", fmt.Errorf("failed to generate Labels clause: %v", err)
	}
	return labels, nil
}

func TestSafetextYamltemplateNegativeNestedTemplate(t *testing.T) {
	innerData = "Negative"
	d := DaemonSetConfiguration{Name: "NestedTest"}
	nestedTmpl, err := template.ParseFS(nestedTemplateYamlTmpl, "nested-template.yaml.tmpl")
	if err != nil {
		t.Fatalf("template.ParseFS() failed: %v", err)
	}
	var buf bytes.Buffer
	err = nestedTmpl.Execute(&buf, &d)
	if err != nil {
		t.Errorf("tmpl.Execute() error = %v", err)
	}
}

func TestSafetextYamltemplatePositiveNestedTemplate(t *testing.T) {
	innerData = "Injection, value: 42"
	d := DaemonSetConfiguration{Name: "NestedTest"}
	nestedTmpl, err := template.ParseFS(nestedTemplateYamlTmpl, "nested-template.yaml.tmpl")
	if err != nil {
		t.Fatalf("template.ParseFS() failed: %v", err)
	}
	var buf bytes.Buffer
	err = nestedTmpl.Execute(&buf, &d)
	if err == nil {
		t.Errorf("tmpl.Execute() error = %v", err)
	}
}

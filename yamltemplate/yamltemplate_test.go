/*
 *
 * Copyright 2022 Google LLC
 *
 * Licensed under the Apache License, Version 2.0 (the "License");
 * you may not use this file except in compliance with the License.
 * You may obtain a copy of the License at
 *
 *     https://www.apache.org/licenses/LICENSE-2.0
 *
 * Unless required by applicable law or agreed to in writing, software
 * distributed under the License is distributed on an "AS IS" BASIS,
 * WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
 * See the License for the specific language governing permissions and
 * limitations under the License.
 *
 */

package yamltemplate_test

import (
	// Replace text/template in your code with safetext/yamltemplate for automatic YAML injection detection

	//"text/template"
	template "github.com/google/safetext/yamltemplate"

	"os"
	"bytes"
	"fmt"
	"strconv"
	"testing"
)

func TestSafetextYamltemplate(t *testing.T) {
	type testCase struct {
		tmplText     string
		replacements map[interface{}]interface{}
		err          error
	}

	testCases := []testCase{
		// Negative cases
		{
			tmplText: "{ hello: \"{{ .addressee | js }}\" }",
			replacements: map[interface{}]interface{}{
				"addressee": "world\", inject: \"oops",
			},
			err: nil,
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
			err:          nil,
		},

		{
			tmplText: `
data:
  HTTPS_PROXY: {{.p1}}
  NO_PROXY: {{.p2}}
`,
			replacements: map[interface{}]interface{}{
				"p1": "1",
				"p2": "localhost, 127.0.0.1",
			},
			err: nil,
		},

		{
			tmplText: `
data:
  HTTPS_PROXY: {{.p1}}
  NO_PROXY: {{.p2}}
`,
			replacements: map[interface{}]interface{}{
				"p1": "",
				"p2": "localhost, 127.0.0.1",
			},
			err: nil,
		},

		{
			tmplText: "{ {{ if not .hide }}hello: {{ .addressee }}{{end}} }",
			replacements: map[interface{}]interface{}{
				"addressee": "world",
				"hide":      false,
			},
			err: nil,
		},

		{
			tmplText: "{ {{ if eq .addressee \"world\" }}hello: {{ .addressee }}{{end}} }",
			replacements: map[interface{}]interface{}{
				"addressee": "world",
			},
			err: nil,
		},

		{
			tmplText: `{ list: "{{ range .entries }}{{.}}{{ end }}" }`,
			replacements: map[interface{}]interface{}{
				"entries": []string{"(special characters to not trigger fast path {})", "two", "three"},
			},
			err: nil,
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
			replacements: map[interface{}]interface{}{
				"some_field": "x",
				"slow":       "{}",
			},
			err: nil,
		},

		{
			tmplText: "{ test: bla }",
			replacements: map[interface{}]interface{}{
				0: "(special characters to not trigger fast path {})",
			},
			err: nil,
		},

		// Verify that unused replacements in nested yaml don't cause templates to fail
		{
			tmplText: `hello:
- to: {{ .addressee }}
  next:
  - first: test
`,
			replacements: map[interface{}]interface{}{
				"addressee": "world",
				"unused":    "some-thing",
			},
			err: nil,
		},

		// Verify that valid strings with non-standard characters work in nested yaml
		{
			tmplText: `hello:
- to: {{ .addressee }}
  next:
  - first: test
`,
			replacements: map[interface{}]interface{}{
				"addressee": "whole-world",
			},
			err: nil,
		},

		// Verify that internal YAML parser rejects duplicate keys
		{
			tmplText: "{ hello: {{ .addressee }}, hello: multiple }",
			replacements: map[interface{}]interface{}{
				"addressee": "world (special characters to not trigger fast path {})",
			},
			err: template.ErrInvalidYAMLTemplate,
		},

		// Verify that internal YAML parsers rejects map keys
		{
			tmplText: "{ {}: {{ .addressee }} }",
			replacements: map[interface{}]interface{}{
				"addressee": "world (special characters to not trigger fast path {})",
			},
			err: template.ErrInvalidYAMLTemplate,
		},

		// Verify that internal YAML parsers rejects slice keys
		{
			tmplText: "{ [1, 2, 3]: {{ .addressee }} }",
			replacements: map[interface{}]interface{}{
				"addressee": "world (special characters to not trigger fast path {})",
			},
			err: template.ErrInvalidYAMLTemplate,
		},

		// Verify that YAML parses still accepts "non-strict" YAML (whilst rejecting duplicate keys)
		{
			tmplText: "a: {{ .addressee }}",
			replacements: map[interface{}]interface{}{
				"addressee": "world (special characters to not trigger fast path {})",
			},
			err: nil,
		},

		// nil type
		{
			tmplText: `{ a: {{.a}}, b: '{{.b}}' }`,
			replacements: map[interface{}]interface{}{
				"a": nil,
				"b": "{}",
			},
			err: nil,
		},

		// Positive cases
		{
			tmplText: "{ hello: \"{{ .addressee }}\" }",
			replacements: map[interface{}]interface{}{
				"addressee": "world\", hello: \"oops_p",
			},
			err: template.ErrYAMLInjection,
		},

		{
			tmplText: "{ hello: \"{{ .addressee }}\", parent: [ 1, {{ .s }}, 3 ] }",
			replacements: map[interface{}]interface{}{
				"addressee": "world",
				"s":         "2, 4",
			},
			err: template.ErrYAMLInjection,
		},

		{
			tmplText: "{ hello: \"{{ .addressee }}\", parent: [ 1, { a: {{ .s }} }, 3 ] }",
			replacements: map[interface{}]interface{}{
				"addressee": "world",
				"s":         "2 , b : b",
			},
			err: template.ErrYAMLInjection,
		},

		{
			tmplText: "{ hello: {{ .addressee }} }",
			replacements: map[interface{}]interface{}{
				"addressee": "{}",
			},
			err: template.ErrYAMLInjection,
		},

		{
			tmplText: "{ {{ if eq .caddressee \"world\" }}hello: {{ .addressee }}{{end}} }",
			replacements: map[interface{}]interface{}{
				"caddressee": "world",
				"addressee":  "world, inject: true",
			},
			err: template.ErrYAMLInjection,
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
			replacements: map[interface{}]interface{}{"addressee": "world\n- inject"},
			err:          template.ErrYAMLInjection,
		},

		// Accessing anchors should count as injected YAML syntax
		{
			tmplText: `{ secret: &secret_label 'test', disclosed: {{ .controlled }}  }`,
			replacements: map[interface{}]interface{}{
				"controlled": "*secret_label",
			},
			err: template.ErrYAMLInjection,
		},
	}

	for _, tc := range testCases {
		tmpl := template.Must(template.New("test").Parse(tc.tmplText))
		var buf bytes.Buffer
		err := tmpl.Execute(&buf, tc.replacements)

		if err != tc.err {
			t.Errorf("Expected %v, got %v\n", tc.err, err)

			if err == nil {
				t.Logf("template execution result was %s\n", buf.String())
			}
		}
	}
}

// Check func maps still work
func sanitize(input interface{}) string {
	return fmt.Sprintf("%q", input)
}

func TestSafetextYamltemplateNegativeFuncMap(t *testing.T) {
	var funcMap = map[string]interface{}{
		"sanitize": sanitize,
	}

	tmpl := template.Must(template.New("test").Funcs(template.FuncMap(funcMap)).Parse(
		"{ a: {{ .a | sanitize }}, b: {{ .b | sanitize }} }",
	))

	replacements := map[string]interface{}{
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

	replacements := map[string]interface{}{
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
	tmplText := `list:
{{with .some_field}}
{{if eq . "x"}}
- {{.}}
{{end}}
{{end}}
`
	f, err := os.CreateTemp("", "list.yaml.tmpl")
	if err != nil {
		t.Errorf("os.CreateTemp() error = %v", err)
	}
	defer os.Remove(f.Name())

	if _, err = f.WriteString(tmplText); err != nil {
		 t.Errorf("f.WriteString() error = %v", err)
	}

	tmpl := template.Must(template.ParseFiles(f.Name()))

	replacements := map[string]interface{}{
		"some_field":    "x",
		"use_slow_path": "{}",
	}

	var buf bytes.Buffer
	err = tmpl.Execute(&buf, replacements)
	if err != nil {
		t.Errorf("tmpl.Execute() error = %v", err)
	}
}

// Check methods work
type A struct {
}

func (A) GetName(n int) string { return "n is " + strconv.Itoa(n) }

func TestSafetextYamltemplateMethod(t *testing.T) {
	tmpl := template.Must(template.New("test").Parse(
		`- {{ (.a.GetName 0x41) | js }}`,
	))

	replacements := map[string]interface{}{
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
	tmpl := template.Must(template.New("test").Parse(
		"{ Person-{{ (StructuralData .Name) }}: {{ .Age }} }",
	))

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

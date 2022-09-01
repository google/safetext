# safetext

**This is not an officially supported Google product.**

Replacements to Golang's `text/template` for specific formats like YAML, that prevent injection vulnerabilities.

## Example use-case

Since `text/template` is not syntax-aware of the formats it produces, it does not offer any protection against injection vulnerabilities.

Consider the following `produceConfig` function which uses `text/template` to generate YAML:

```
package main

import (
        "bytes"
        "fmt"
        "text/template"
)

func produceConfig(params any) (error, string) {
        tmpl, _ := template.New("test").Parse("{ hello: {{ .addressee }} }")

        var buf bytes.Buffer
        err := tmpl.Execute(&buf, params)
        if err != nil {
                return err, ""
        }

        return nil, buf.String()
}

func main() {
        goodReplacements := map[string]interface{}{
                "addressee": "safe",
        }

        err, config := produceConfig(goodReplacements)

        if err == nil {
                fmt.Println(config)
        } else {
                fmt.Printf("Error: %v\n", err)
        }

        badReplacements := map[string]interface{}{
                "addressee": "world, oops: true",
        }

        err, config = produceConfig(badReplacements)

        if err == nil {
                fmt.Println(config)
        } else {
                fmt.Printf("Error: %v\n", err)
        }
}
```

This program demonstrates how a malicious `addressee` input can cause injection of new YAML keys in the template execution result.

With `text/template`, no errors will be encountered when this happens, and the program output will be:

```
{ hello: safe }
{ hello: world, oops: true }
```

By instead switching from `text/template` to `safetext/yamltemplate`, the injection would have been prevented, with the output instead being:

```
{ hello: safe }
Error: YAML Injection Detected
```

## Unsupported use-cases common to all formats:

-   Escaping logic outside of the templating system. Instead, you should
    annotate the escaping logic into your template (EG: `.UntrustedField |
    escape`).

-   Data accessed indirectly (private struct fields, or string table lookups).
    The libraries do not scan private struct fields, so if you expose them
    indirectly (EG: `.Object | GetPrivateString`) you will not be protected
    against a potential injection. Similarly, accessing data through a string
    table lookup function (EG: `{{ (GetAssociatedObjectWithName .Name) | GetY
    }}`) is unsupported for the additional reason that the libraries work by
    performing executions with mutated string inputs.

-   Functions with side effects. The libraries work by performing multiple
    template executions, so if you register functions that have side effects,
    this could cause unexpected behaviour (EG: `id: {{ AllocateID }}`).

-   Partial formats. The libraries are designed to be used for generating
    complete files. If you generate segments and then concatenate them together,
    you should instead move this logic into the templating system itself (using
    constructs like `if` or `range`).

To opt-out specific variables from injection detection, you can use the special
function, `StructuralData`, which will leave all direct arguments as-is. For
example:

```
{ Person-{{ (StructuralData .Name) }}: {{ .Age }} }
```

## yamltemplate

Unsupported use cases:

-   YAML with duplicate keys. Duplicate keys are non-standard YAML, and not supported by this library.
    Please refactor your YAML template to remove duplicate keys. For example:

    ```
    - project:
       members: member-a
       members: member-b
    ```

    To:

    ```
    - project:
      members: member-b
    ```

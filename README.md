# safetext

**This is not an officially supported Google product.**

Safe-by-construction libraries for producing formats like YAML, to replace
syntax-unaware libraries like `text/template` and `sprintf` that are at risk of
injection vulnerabilities.

## Example use-case

Since `text/template` is not syntax-aware of the formats it produces, it does
not offer any protection against injection vulnerabilities.

Consider the following `produceConfig` function which uses `text/template` to
generate YAML:

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

This program demonstrates how a malicious `addressee` input can cause injection
of new YAML keys in the template execution result.

With `text/template`, no errors will be encountered when this happens, and the
program output will be:

```
{ hello: safe }
{ hello: world, oops: true }
```

By instead switching from `text/template`to `safetext/yamltemplate`, the
injection would have been prevented, with the output instead being:

```
{ hello: safe }
Error: YAML Injection Detected
```

## Instructions for `text/template` replacements

Injection detection is automatically applied when accessing input data fields.

-   It can also be manually enabled on the result of any function call:

    ```
    {{ RetrieveUntrustedData | ApplyInjectionDetection }}
    ```

-   The injection logic can be disabled on certain fields by applying the
    `StructuralData` annotation:

    ```
    {{ (StructuralData .x) }}
    ```

-   The `StructuralData` annotation is also needed when passing an input to a
    function where the input should not be mutated, such as a performing some
    kind of lookup:

    ```
    name: {{ readFile (StructuralData .pathToName) | ApplyInjectionDetection }}
    ```

-   It is recommended to make full use of `text/template` features like
    conditional expressions, range loops, etc to avoid the `StructuralData`
    annotation where possible. For example, instead of:

    ```
    properties:
        {{ (StructuralData .PropertiesYaml) }}
    ```

    Consider:

    ```
    properties:{{ range .Properties }}
        - {{ . }}{{ end }}
    ```

### `yamltemplate`

The intention of `yamltemplate` is to ensure that by-default none of the strings
in the input data affect the structure of the resultant YAML (just the values).

-   For example, the below template would be compatible with yamltemplate as-is,
    whilst automatically preventing any injections from the `Name` input:

    ```
    name: {{.Name}}
    ```

-   However, any template nodes that *are* expected to change the resultant YAML
    structure, such as inserting arbitrary YAML config, would need to be
    annotated explicitly as `StructuralData`:

    ```
    config: {{ (StructuralData .Config) }}
    ```

-   Another case of needing the `StructuralData` annotation would be where you
    need to include a complete map into the yaml structure. Using
    `StructuralData` alone may let injections pass through via the key so we
    need an extra layer of validation here:

    ~~~
    labels:
    {{- range $key, $value := .Labels }}
        {{ (StructuralData $key | MapKey) }}: {{ $value }}
    {{- end }}
     ```

    The corresponding golang side could look like this:

    ~~~

    func mapKeyFunc(data any) (string, error) { if v, ok := data.(string); ok {
    matched, err := regexp.MatchString(`^[a-zA-Z0-9/\-.]+$`, v) if err != nil {
    return "", err } if !matched { return "", fmt.Errorf("invalid characters in
    the key: %v", v) } return v, nil }

    ```
    return "", errors.New("invalid input")
    ```

    } ...

    tmp:= template.New("something") tmp.Funcs(map[string]any{"MapKey":
    mapKeyFunc}) tmpl := template.Must(tmp.Parse(yamlTemplate)) ```

-   You can combine `yamltemplate` with `shprintf`. Consider the following
    cloud-init yaml template:

    ```
    ---
    write_files:
    - path: /etc/nginx/refresh.sh
      owner: root:root
      permissions: 0755  # Don't forget the 0 (you are probably using octal...)
      content: |
        #!/bin/bash
        set -euo pipefail

        {{ shprintf `curl %s > /tmp/something` .userInput }}
    ```

    Evaluating this template with safetext/yamltemplate, both shell command and
    YAML injections will be prevented.

    To do this, you need to setup the golang side like this:

    ```
    tmp:= addons.WithShsprintf(template.New("something"))
    tmpl := template.Must(tmp.Parse(yamlTemplate))
    ```

#### Unsupported use cases for `yamltemplate`

-   YAML with duplicate keys. Duplicate keys are non-standard YAML, and not
    supported by this library. Please refactor your YAML template to remove
    duplicate keys. For example:

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

### `shtemplate`

`shtemplate` is designed to allow you to generate shell scripts with the
guarantee that none of the input data strings will be able to inject new
commands or flags, without explicit annotation.

-   For example, a template script designed to just print one string will fail
    to render if that string injects a new command `` `./evil` ``:

    ```
    echo "{{ .addressee }}"
    ```

-   To explicitly allow an input string to contain new commands not from the
    template string, the `StructuralData` annotation can be used:

    ```
    {{ (StructuralData .commands) }}
    ```

-   Flags (arguments starting with `-`) are also forbidden by-default. For
    example, the below template will fail to render if `Filename` is
    `--interactive`:

    ```
    git add {{ .Filename }}
    ```

-   To explicitly allow an input string passed as a command argument to be a
    flag, the `AllowFlags` annotation can be used:

    ```
    git add {{ (AllowFlags .FilenameOrGitAddFlag) }}
    ```

-   Multiple arguments from a single input string is also forbidden by-default.
    This construct should instead be implemented using an array and `range`
    expression:

    ```
    ls {{ range .Paths }}{{.}} {{end}}
    ```

### Unsupported use-cases for `text/template` replacements

-   Escaping logic outside of the templating system. Instead, you should
    annotate the escaping logic into your template (EG: `.UntrustedField |
    escape`).

-   Partial formats. The libraries are designed to be used for generating
    complete files. If you generate segments and then concatenate them together,
    you should instead move this logic into the templating system itself (using
    constructs like `if` or `range`).

-   Functions with side effects. The libraries work by performing multiple
    template executions, so if you register functions that have side effects,
    this could cause unexpected behaviour (EG: `id: {{ AllocateID }}`).

## `shsprintf`

`shsprintf` is designed to allow you to generate shell scripts with the
guarantee that none of the input data strings will be able to inject new
commands or flags regardless of potentially incorrect escaping. See the below
example, which will return the error `shsprintf.ErrShInjection` instead of the
script with an injected command:

```
message := "`whoami`"
result, err := shsprintf.Sprintf("git commit -m %s", message)
```

`shsprintf.Sprintf` adds an error return value compared to `fmt.Sprintf`, but
the API is otherwise the same. `shsprintf.MustSprintf` is available for cases
where panic is acceptable.

`shsprintf` comes with an escaping function that is recommended for use:

```
message := "`whoami`"
result := shsprintf.MustSprintf("git commit -m %s", shsprintf.EscapeDefaultContext(message))
```

Unlike with `text/template` there are no special annotations. If you need to
pass multiple arguments for example, this should be done by altering the format
string:

```
files := []any{ "file1", "file2", "file3" }
result, err := shsprintf.Sprintf("cat" + strings.Repeat(" %s", len(files)), files...)
```

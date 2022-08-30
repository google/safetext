licenses(["notice"])  # Apache 2.0

load("@io_bazel_rules_go//go:def.bzl", "go_binary", "go_library", "go_test")

load("@bazel_gazelle//:def.bzl", "gazelle")

# gazelle:prefix github.com/google/safetext
gazelle(name = "gazelle")

package(default_visibility = ["//visibility:public"])

go_library(
    name = "common",
    importpath = "github.com/google/safetext/common",
    srcs = ["common/common.go"],
    visibility = ["//visibility:private"],
    deps = [],
)

go_library(
    name = "yamltemplate",
    importpath = "github.com/google/safetext/yamltemplate",
    srcs = ["yamltemplate/yamltemplate.go"],
    deps = [
        ":common",
        "@in_gopkg_yaml_v3//:yaml_v3"
    ],
)

go_test(
    name = "yamltemplate_test",
    size = "small",
    srcs = ["yamltemplate/yamltemplate_test.go"],
    data = [":testdata/list.yaml.tmpl"],
    deps = [
        ":yamltemplate",
    ],
)

licenses(["notice"])  # Apache 2.0

load("@bazel_gazelle//:def.bzl", "gazelle")

# gazelle:prefix github.com/google/safetext
# gazelle:go_naming_convention import_alias
gazelle(name = "gazelle")

load("@com_github_bazelbuild_buildtools//buildifier:def.bzl", "buildifier")

buildifier(
    name = "buildifier",
)

load("@bazel_gazelle//:deps.bzl", "go_repository")
def go_repositories():
    go_repository(
        name = "cc_mvdan_sh_v3",
        build_file_proto_mode = "disable_global",
        importpath = "mvdan.cc/sh/v3",
        sum = "h1:hmP3UOw4f+EYexsJjFxvU38+kn+V/s2CclXHanIBkmQ=",
        version = "v3.5.1",
    )
    go_repository(
        name = "com_github_bazelbuild_bazelisk",
        build_file_proto_mode = "disable_global",
        importpath = "github.com/bazelbuild/bazelisk",
        sum = "h1:K7s8deYR2/+qvkNyUVyKeBJIqQ3odVthOrMGJTF7TdI=",
        version = "v1.12.2",
    )
    go_repository(
        name = "com_github_bazelbuild_rules_go",
        build_file_proto_mode = "disable_global",
        importpath = "github.com/bazelbuild/rules_go",
        sum = "h1:cmObMtgIOaEU944SqXtJ9DnlS8IPGGa7pdRnsrpQzXM=",
        version = "v0.34.0",
    )
    go_repository(
        name = "com_github_hashicorp_go_version",
        build_file_proto_mode = "disable_global",
        importpath = "github.com/hashicorp/go-version",
        sum = "h1:feTTfFNnjP967rlCxM/I9g701jU+RN74YKx2mOkIeek=",
        version = "v1.6.0",
    )
    go_repository(
        name = "com_github_jdxcode_netrc",
        build_file_proto_mode = "disable_global",
        importpath = "github.com/jdxcode/netrc",
        sum = "h1:d4+I1YEKVmWZrgkt6jpXBnLgV2ZjO0YxEtLDdfIZfH4=",
        version = "v0.0.0-20210204082910-926c7f70242a",
    )
    go_repository(
        name = "com_github_kr_pty",
        build_file_proto_mode = "disable_global",
        importpath = "github.com/kr/pty",
        sum = "h1:VkoXIwSboBpnk99O/KFauAEILuNHv5DVFKZMBN/gUgw=",
        version = "v1.1.1",
    )
    go_repository(
        name = "com_github_kr_text",
        build_file_proto_mode = "disable_global",
        importpath = "github.com/kr/text",
        sum = "h1:45sCR5RtlFHMR4UwH9sdQ5TC8v0qDQCHnXt+kaKSTVE=",
        version = "v0.1.0",
    )
    go_repository(
        name = "com_github_mitchellh_go_homedir",
        build_file_proto_mode = "disable_global",
        importpath = "github.com/mitchellh/go-homedir",
        sum = "h1:lukF9ziXFxDFPkA1vsr5zpc1XuPDn/wFntq5mG+4E0Y=",
        version = "v1.1.0",
    )
    go_repository(
        name = "com_github_niemeyer_pretty",
        build_file_proto_mode = "disable_global",
        importpath = "github.com/niemeyer/pretty",
        sum = "h1:fD57ERR4JtEqsWbfPhv4DMiApHyliiK5xCTNVSPiaAs=",
        version = "v0.0.0-20200227124842-a10e7caefd8e",
    )
    go_repository(
        name = "in_gopkg_check_v1",
        build_file_proto_mode = "disable_global",
        importpath = "gopkg.in/check.v1",
        sum = "h1:QRR6H1YWRnHb4Y/HeNFCTJLFVxaq6wH4YuVdsUOr75U=",
        version = "v1.0.0-20200902074654-038fdea0a05b",
    )
    go_repository(
        name = "in_gopkg_yaml_v3",
        build_file_proto_mode = "disable_global",
        importpath = "gopkg.in/yaml.v3",
        sum = "h1:fxVm/GzAzEWqLHuvctI91KS9hhNmmWOoWu0XTYJS7CA=",
        version = "v3.0.1",
    )

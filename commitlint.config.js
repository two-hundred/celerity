module.exports = {
  extends: ["@commitlint/config-conventional"],
  rules: {
    "type-enum": [
      2,
      "always",
      [
        "fix",
        "build",
        "revert",
        "wip",
        "feat",
        "chore",
        "ci",
        "docs",
        "style",
        "refactor",
        "perf",
        "test",
        "instr"
      ],
    ],
    "scope-enum": [
      2,
      "always",
      [
        "blueprint",
        "build-engine",
        "common",
        "runtime-core",
        "runtime-go",
        "runtime-java",
        "runtime-nodejs",
        "runtime-python",
        "runtime-rust",
        "api",
        "cli",
        "tool-releaser",
        "tool-test-runner",
        "validator-go",
        "validator-java",
        "validator-nodejs",
        "validator-python",
        "validator-rust",
        "templates-go",
        "templates-java",
        "templates-nodejs",
        "templates-python",
        "templates-rust"
      ],
    ],
  },
};

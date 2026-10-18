module.exports = {
  branches: ["main"],
  repositoryUrl: "https://github.com/imrenagicom/adsctl.git",
  tagFormat: "v${version}",
  plugins: [
    ["@semantic-release/commit-analyzer", { preset: "conventionalcommits" }],
    ["@semantic-release/release-notes-generator", { preset: "conventionalcommits" }],
    [
      "@semantic-release/exec",
      {
        verifyConditionsCmd: "make release-check",
        // Sync plugin manifest versions, commit, build archives before semantic-release tags HEAD,
        // and push the release commit so Git installs and release archives share the same version.
        prepareCmd: [
          "python3 scripts/check-plugin.py --set-version ${nextRelease.version}",
          'git add plugin.json .claude-plugin/plugin.json .codex-plugin/plugin.json && (git diff --cached --quiet || git commit -m "chore(release): ${nextRelease.version} [skip ci]")',
          "GORELEASER_CURRENT_TAG=${nextRelease.gitTag} goreleaser release --clean --skip=publish,validate",
          "python3 scripts/check-plugin.py --archives dist --version ${nextRelease.version}",
          "git push origin HEAD:${branch.name}",
        ].join(" && "),
      },
    ],
    [
      "@semantic-release/github",
      {
        assets: ["dist/*.tar.gz", "dist/*.zip", "dist/checksums.txt", "install.sh"],
        successCommentCondition: false,
        failCommentCondition: false,
        releasedLabels: false,
      },
    ],
  ],
};

# Codex and Claude Code plugin

The `adsctl` plugin bundles three shared skills: `adsctl-setup` for CLI installation and Meta credentials, `adsctl-meta-analyst` for structured campaign performance audits and funnel diagnostics, and `adsctl` for reporting, monitoring, and authorized management. The repository root is the plugin root. Host-specific manifests point to the same `skills/` tree; no MCP server or connector is required.

Use a local host with shell execution. The setup skill installs the CLI through the release installer and guides credential provisioning. On macOS/Linux:

```sh
curl -fsSL https://github.com/imrenagicom/adsctl/releases/latest/download/install.sh | sh
source "$HOME/.adsctl/env"
```

The default home holds `bin/adsctl`, `config.yaml`, optional secret files, and state. `ADSCTL_HOME` overrides that root. The generated `env` file configures PATH and contains no credentials. Installing plugin skills alone does not grant Meta permissions. Windows supports ordinary commands, but persistent launch/monitor execution requires macOS/Linux; Windows binary installation uses the release ZIP and manual PATH setup.

## Codex

From an adsctl checkout or an extracted release containing the plugin manifests:

```sh
codex plugin marketplace add /absolute/path/to/adsctl
codex plugin add adsctl@adsctl
codex plugin list --marketplace adsctl --json
```

After these files are published to GitHub, the repository can also be registered directly:

```sh
codex plugin marketplace add imrenagicom/adsctl
codex plugin add adsctl@adsctl
```

In Codex desktop, select the adsctl plugin from the local marketplace and install it. Start a new chat or restart the app if the skills do not appear. Run the plugin's setup workflow, or ask it to use `adsctl-setup` to install the executable and configure Meta access. The manifest declares local execution and the setup skill as onboarding. Marketplace distribution is separate from publication to OpenAI's public plugin directory.

## Claude Code

From the checkout or extracted plugin package:

```sh
claude plugin marketplace add /absolute/path/to/adsctl
claude plugin install adsctl@adsctl
claude plugin details adsctl
```

After publication, use `claude plugin marketplace add imrenagicom/adsctl` instead of the local path. Inside Claude Code, the equivalent commands begin with `/plugin marketplace add` and `/plugin install`. Start a new session, then invoke `/adsctl:adsctl-setup` for setup, `/adsctl:adsctl-meta-analyst` for campaign performance audits, or `/adsctl:adsctl` for operations. For temporary development without installation, use `claude --plugin-dir /absolute/path/to/adsctl`.

## Verify and maintain the package

Run maintainer checks from a source checkout:

```sh
make plugin-check
make installer-check
claude plugin validate .claude-plugin/plugin.json --strict
claude plugin validate .claude-plugin/marketplace.json --strict
claude plugin validate skills --strict
make release-snapshot
```

`plugin-check` needs Python 3.9 or later and checks identity/version consistency, both marketplace paths, onboarding, and packaged skill references. `release-snapshot` also verifies that all six archives contain both manifests, both catalogs, the CLI, and the complete skill resources. Claude's native validator and Codex's marketplace discovery provide host-specific checks; the Python check is a packaging check, not a substitute for either host parser.

Native inventory checks were performed with Codex CLI 0.162.1 and Claude Code 2.1.293. Both discovered `adsctl` and `adsctl-setup`; Claude's local marketplace installation was exercised with temporary settings. These checks do not call Meta or establish credential access.

The plugin has an independent version in both host manifests. Bump both together when changing the plugin skills, manifests, or bundled resources so cached Git installs can receive the update. CLI versions continue to come from release tags. Refresh the registered marketplace and update/reinstall the plugin after publishing a plugin version change. Earlier CLI releases without these manifests are standalone CLI/skill distributions.

The layouts follow the official [Codex plugin packaging](https://developers.openai.com/plugins/build/plugins), [desktop-only plugin](https://developers.openai.com/plugins/deploy/desktop-only), [Claude plugin manifest](https://code.claude.com/docs/en/plugins-reference), and [Claude marketplace](https://code.claude.com/docs/en/plugin-marketplaces) documentation.

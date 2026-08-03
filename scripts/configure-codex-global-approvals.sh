#!/usr/bin/env bash

set -euo pipefail

readonly RULES_MARKER="# Managed by configure-codex-global-approvals.sh"

function usage() {
	echo "Usage: $0 [--codex-home DIR] [--gopls-server NAME]"
	echo ""
	echo "Globally allows git add/commit/diff/status/log and auto-approves all"
	echo "tools exposed by an existing gopls or gopls-mcp Codex MCP server."
}

function fail() {
	echo "Error: $*" >&2
	exit 1
}

function parse_args() {
	codex_dir="${CODEX_HOME:-${HOME:?HOME is not set}/.codex}"
	gopls_server=""

	while (($# > 0)); do
		case "$1" in
		--codex-home)
			(($# >= 2)) || fail "--codex-home requires a directory"
			codex_dir=$2
			shift 2
			;;
		--gopls-server)
			(($# >= 2)) || fail "--gopls-server requires a server name"
			gopls_server=$2
			shift 2
			;;
		--help | -h)
			usage
			exit 0
			;;
		*)
			fail "unknown argument: $1"
			;;
		esac
	done
}

function install_git_rules() {
	local rules_dir="$codex_dir/rules"
	local rules_file="$rules_dir/allow-git-commands.rules"

	mkdir -p "$rules_dir"

	if [[ -e "$rules_file" ]] && ! grep -Fq "$RULES_MARKER" "$rules_file"; then
		fail "$rules_file already exists and is not managed by this script"
	fi

	local temp_file
	temp_file=$(mktemp "$rules_dir/.allow-git-commands.rules.XXXXXX")
	cat >"$temp_file" <<'RULES'
# Managed by configure-codex-global-approvals.sh
# Codex evaluates each literal argument after these prefixes separately.
prefix_rule(
    pattern = ["git", ["add", "commit", "diff", "status", "log"]],
    decision = "allow",
    justification = "User-approved Git staging, commits, and repository inspection",
    match = [
        "git add .",
        "git add path/to/file",
        "git commit -m example",
        "git diff --cached",
        "git status --short",
        "git log --oneline -5",
    ],
    not_match = [
        "git push",
        "git reset --hard",
    ],
)
RULES
	chmod 600 "$temp_file"
	mv "$temp_file" "$rules_file"
	echo "Updated Git approvals: $rules_file"
}

function table_exists() {
	local server=$1
	grep -Fqx "[mcp_servers.$server]" "$config_file" ||
		grep -Fqx "[mcp_servers.\"$server\"]" "$config_file"
}

function resolve_gopls_server() {
	if [[ -n "$gopls_server" ]]; then
		table_exists "$gopls_server" ||
			fail "MCP server '$gopls_server' is not configured in $config_file"
		return
	fi

	if table_exists "gopls-mcp"; then
		gopls_server="gopls-mcp"
	elif table_exists "gopls"; then
		gopls_server="gopls"
	else
		fail "no gopls-mcp or gopls MCP server is configured in $config_file"
	fi
}

function preflight_gopls_config() {
	config_file="$codex_dir/config.toml"
	[[ -f "$config_file" ]] ||
		fail "$config_file does not exist; configure the gopls MCP server first"

	resolve_gopls_server
}

function update_gopls_approvals() {

	local temp_file
	temp_file=$(mktemp "$codex_dir/.config.toml.XXXXXX")

	awk -v server="$gopls_server" '
    function finish_target() {
      if (!saw_enabled) print "enabled = true"
      if (!saw_approval) print "default_tools_approval_mode = \"approve\""
    }

    BEGIN {
      plain_header = "[mcp_servers." server "]"
      quoted_header = "[mcp_servers.\"" server "\"]"
    }

    /^\[/ {
      if (in_target) finish_target()
      in_target = ($0 == plain_header || $0 == quoted_header)
      if (in_target) {
        found = 1
        saw_enabled = 0
        saw_approval = 0
      }
    }

    in_target && /^[[:space:]]*enabled[[:space:]]*=/ {
      print "enabled = true"
      saw_enabled = 1
      next
    }

    in_target && /^[[:space:]]*default_tools_approval_mode[[:space:]]*=/ {
      print "default_tools_approval_mode = \"approve\""
      saw_approval = 1
      next
    }

    in_target && /^[[:space:]]*(enabled_tools|disabled_tools)[[:space:]]*=/ {
      if ($0 !~ /]/) skipping_array = 1
      next
    }

    skipping_array {
      if ($0 ~ /]/) skipping_array = 0
      next
    }

    { print }

    END {
      if (in_target) finish_target()
      if (!found) exit 42
    }
  ' "$config_file" >"$temp_file" || {
		local status=$?
		rm -f "$temp_file"
		if [[ $status -eq 42 ]]; then
			fail "MCP server '$gopls_server' disappeared while updating $config_file"
		fi
		exit "$status"
	}

	chmod 600 "$temp_file"
	mv "$temp_file" "$config_file"
	echo "Updated all-tool approvals for MCP server '$gopls_server': $config_file"
}

function main() {
	parse_args "$@"
	mkdir -p "$codex_dir"
	preflight_gopls_config
	install_git_rules
	update_gopls_approvals
	echo "Restart Codex for the changes to take effect."
}

main "$@"

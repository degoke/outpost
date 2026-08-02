#!/usr/bin/env bash
# Install Outpost from GitHub Releases.
#
# Usage:
#   curl -fsSL https://raw.githubusercontent.com/degoke/outpost/main/scripts/install.sh | bash
#
# Environment:
#   OUTPOST_VERSION   Release tag (e.g. v0.1.0) or "latest" (default)
#   OUTPOST_INSTALL_DIR  Install directory (default: $HOME/.local/bin)
#   OUTPOST_REPO      GitHub repo (default: degoke/outpost)

set -euo pipefail

REPO="${OUTPOST_REPO:-degoke/outpost}"
VERSION="${OUTPOST_VERSION:-latest}"
INSTALL_DIR="${OUTPOST_INSTALL_DIR:-${HOME}/.local/bin}"
BINARY_NAME="outpost"

info() {
	printf '==> %s\n' "$*"
}

warn() {
	printf 'warning: %s\n' "$*" >&2
}

die() {
	printf 'error: %s\n' "$*" >&2
	exit 1
}

need_cmd() {
	command -v "$1" >/dev/null 2>&1 || die "missing required command: $1"
}

detect_platform() {
	local os arch
	os="$(uname -s)"
	arch="$(uname -m)"

	case "${os}" in
	Linux) os="linux" ;;
	Darwin) os="darwin" ;;
	MINGW* | MSYS* | CYGWIN* | Windows_NT)
		die "Windows is not supported by this script — download outpost_${VERSION}_windows_amd64.zip from GitHub Releases"
		;;
	*)
		die "unsupported operating system: ${os}"
		;;
	esac

	case "${arch}" in
	x86_64 | amd64) arch="amd64" ;;
	aarch64 | arm64) arch="arm64" ;;
	*)
		die "unsupported CPU architecture: ${arch}"
		;;
	esac

	printf '%s %s\n' "${os}" "${arch}"
}

resolve_version() {
	if [ "${VERSION}" = "latest" ]; then
		need_cmd curl
		local tag
		tag="$(
			curl -fsSL "https://api.github.com/repos/${REPO}/releases/latest" |
				tr -d '\n' |
				sed -n 's/.*"tag_name"[[:space:]]*:[[:space:]]*"\([^"]*\)".*/\1/p'
		)"
		[ -n "${tag}" ] || die "could not determine latest release for ${REPO}"
		printf '%s\n' "${tag}"
		return
	fi
	case "${VERSION}" in
	v*) printf '%s\n' "${VERSION}" ;;
	*) printf 'v%s\n' "${VERSION}" ;;
	esac
}

archive_name() {
	local version="$1"
	local os="$2"
	local arch="$3"
	version="${version#v}"
	if [ "${os}" = "windows" ]; then
		printf 'outpost_%s_%s_%s.zip\n' "${version}" "${os}" "${arch}"
		return
	fi
	printf 'outpost_%s_%s_%s.tar.gz\n' "${version}" "${os}" "${arch}"
}

download() {
	local url="$1"
	local dest="$2"
	curl -fsSL --retry 3 --retry-delay 1 -o "${dest}" "${url}"
}

sha256_file() {
	local file="$1"
	if command -v sha256sum >/dev/null 2>&1; then
		sha256sum "${file}" | awk '{print $1}'
	elif command -v shasum >/dev/null 2>&1; then
		shasum -a 256 "${file}" | awk '{print $1}'
	else
		die "need sha256sum or shasum to verify downloads"
	fi
}

verify_checksum() {
	local archive="$1"
	local checksums="$2"
	local base expected actual

	base="$(basename "${archive}")"
	expected="$(awk -v f="${base}" '$2 == f { print $1; exit }' "${checksums}")"
	[ -n "${expected}" ] || die "no checksum entry for ${base}"
	actual="$(sha256_file "${archive}")"
	if [ "${actual}" != "${expected}" ]; then
		die "checksum mismatch for ${base}"
	fi
}

extract_binary() {
	local archive="$1"
	local dest_dir="$2"
	local os="$3"

	case "${archive}" in
	*.tar.gz)
		if tar -tzf "${archive}" | awk '
			BEGIN { bad=0 }
			/^\// || /(^|\/)\.\.($|\/)/ { bad=1 }
			END { exit bad }
		'; then
			tar --no-same-owner -xzf "${archive}" -C "${dest_dir}"
		else
			die "archive contains an unsafe path"
		fi
		;;
	*.zip)
		need_cmd unzip
		if unzip -Z1 "${archive}" | awk '
			BEGIN { bad=0 }
			/^\// || /(^|\/)\.\.($|\/)/ { bad=1 }
			END { exit bad }
		'; then
			unzip -qo "${archive}" -d "${dest_dir}"
		else
			die "archive contains an unsafe path"
		fi
		;;
	*)
		die "unsupported archive format: ${archive}"
		;;
	esac

	if [ "${os}" = "windows" ]; then
		BINARY_NAME="outpost.exe"
	fi
	[ -f "${dest_dir}/${BINARY_NAME}" ] || die "binary not found in archive"
}

install_binary() {
	local src="$1"
	mkdir -p "${INSTALL_DIR}"
	if [ -w "${INSTALL_DIR}" ]; then
		install -m 0755 "${src}" "${INSTALL_DIR}/${BINARY_NAME}"
		return
	fi
	if command -v sudo >/dev/null 2>&1; then
		info "installing to ${INSTALL_DIR} (sudo required)"
		sudo install -m 0755 "${src}" "${INSTALL_DIR}/${BINARY_NAME}"
		return
	fi
	die "cannot write to ${INSTALL_DIR} — set OUTPOST_INSTALL_DIR to a writable directory"
}

path_hint() {
	case ":${PATH}:" in
	*":${INSTALL_DIR}:"*) return ;;
	esac
	warn "${INSTALL_DIR} is not on your PATH"
	printf 'Add it with:\n  export PATH="%s:$PATH"\n' "${INSTALL_DIR}"
}

main() {
	need_cmd curl
	need_cmd tar

	read -r os arch < <(detect_platform)
	tag="$(resolve_version)"
	archive="$(archive_name "${tag}" "${os}" "${arch}")"
	base_url="https://github.com/${REPO}/releases/download/${tag}"

	tmpdir="$(mktemp -d)"
	trap 'rm -rf "${tmpdir}"' EXIT

	archive_path="${tmpdir}/${archive}"
	checksums_path="${tmpdir}/checksums.txt"

	info "installing outpost ${tag} for ${os}/${arch}"
	download "${base_url}/${archive}" "${archive_path}"

	download "${base_url}/checksums.txt" "${checksums_path}" || die "release checksums.txt is required"
	verify_checksum "${archive_path}" "${checksums_path}"

	extract_binary "${archive_path}" "${tmpdir}" "${os}"
	install_binary "${tmpdir}/${BINARY_NAME}"

	info "installed ${BINARY_NAME} to ${INSTALL_DIR}/${BINARY_NAME}"
	if command -v "${BINARY_NAME}" >/dev/null 2>&1; then
		"${BINARY_NAME}" --help >/dev/null 2>&1 || true
		info "run: ${BINARY_NAME} --help"
	else
		path_hint
	fi
}

main "$@"

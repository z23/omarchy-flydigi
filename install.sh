#!/bin/bash
# Sets up the Flydigi Vader 4 plugin: builds the vader4-rgb helper from the Go
# source in helper/, installs it to ~/.local/bin, and installs a udev rule so
# the logged-in user can open the controller without root.
#
#   ./install.sh               build, install the helper, install the udev rule
#   ./install.sh --no-udev     skip the udev rule (and sudo)
#
# VADER4_BIN_DIR overrides where the helper goes. Nothing is written inside the
# plugin folder, so the shell does not reload the plugin while this runs.
set -euo pipefail

here=$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)
bin_dir=${VADER4_BIN_DIR:-$HOME/.local/bin}
rule_src=$here/udev/60-flydigi.rules
rule_dst=/etc/udev/rules.d/60-flydigi.rules
plugin_id=io.github.z23.flydigi
with_udev=1

for arg in "$@"; do
  case $arg in
    --no-udev) with_udev=0 ;;
    -h | --help)
      sed -n '2,10p' "$0" | sed 's/^# \{0,1\}//'
      exit 0
      ;;
    *)
      echo "install.sh: unknown option $arg" >&2
      exit 2
      ;;
  esac
done

step() { printf '\n\033[1m%s\033[0m\n' "$*"; }

# Rules without their comments, to tell whether the installed file is current.
rules_of() { grep -v -e '^[[:space:]]*#' -e '^[[:space:]]*$' "$1" 2>/dev/null || true; }

if ! command -v go >/dev/null 2>&1; then
  step "Installing Go to build the helper"
  sudo pacman -S --needed go
fi

step "Building vader4-rgb"
tmp=$(mktemp -d)
trap 'rm -rf "$tmp"' EXIT
(cd "$here/helper" && CGO_ENABLED=0 go build -trimpath -ldflags "-s -w" -o "$tmp/vader4-rgb" .)
"$tmp/vader4-rgb" --version

target=$bin_dir/vader4-rgb
if [[ -x $target ]] && ! "$target" --version 2>/dev/null | grep -q 'omarchy-flydigi'; then
  # An older vader4-rgb from z23/flydigictl has the same name.
  echo "Keeping the older $target as $target.old"
  mv -f "$target" "$target.old"
fi
install -Dm755 "$tmp/vader4-rgb" "$target"
echo "Installed $target"
case ":$PATH:" in
  *":$bin_dir:"*) ;;
  *) echo "Note: $bin_dir is not on PATH. The plugin finds the helper there anyway." ;;
esac

if ((with_udev)); then
  if [[ "$(rules_of "$rule_src")" == "$(rules_of "$rule_dst")" ]]; then
    step "The udev rule is already installed"
  else
    step "Installing the udev rule (needs sudo)"
    sudo install -Dm644 "$rule_src" "$rule_dst"
    sudo udevadm control --reload
    sudo udevadm trigger --action=change --subsystem-match=usb --subsystem-match=hidraw
    echo "Installed $rule_dst"
  fi
fi

# Tell a running shell to look for the helper again. Quiet if it is not running
# or the plugin is not enabled yet.
omarchy-shell -q "$plugin_id" recheck >/dev/null 2>&1 || true

step "Done"
"$target" status || true
enabled=$(omarchy plugin list --json 2>/dev/null | jq -r --arg id "$plugin_id" '.[] | select(.id == $id) | .enabled' 2>/dev/null || true)
if [[ $enabled == false ]]; then
  echo
  echo "Enable the bar widget with: omarchy plugin enable $plugin_id"
fi
echo
echo "If the panel still says permission is needed, unplug the controller and plug it back in."

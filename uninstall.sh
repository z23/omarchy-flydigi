#!/bin/bash
# Removes what install.sh added: the vader4-rgb helper, the udev rule, and the
# helper's state file. Run it before `omarchy plugin remove io.github.z23.flydigi`,
# which deletes this folder but never runs anything from it.
#
#   ./uninstall.sh             remove the helper, state file and udev rule
#   ./uninstall.sh --keep-udev leave the udev rule (and skip sudo)
set -euo pipefail

bin_dir=${VADER4_BIN_DIR:-$HOME/.local/bin}
rule_dst=/etc/udev/rules.d/60-flydigi.rules
state_dir=${XDG_STATE_HOME:-$HOME/.local/state}/vader4-rgb
keep_udev=0

for arg in "$@"; do
  case $arg in
    --keep-udev) keep_udev=1 ;;
    -h | --help)
      sed -n '2,8p' "$0" | sed 's/^# \{0,1\}//'
      exit 0
      ;;
    *)
      echo "uninstall.sh: unknown option $arg" >&2
      exit 2
      ;;
  esac
done

target=$bin_dir/vader4-rgb
if [[ -x $target ]] && "$target" --version 2>/dev/null | grep -q 'omarchy-flydigi'; then
  rm -f "$target"
  echo "Removed $target"
  if [[ -x $target.old ]]; then
    mv -f "$target.old" "$target"
    echo "Restored the older vader4-rgb that install.sh set aside"
  fi
elif [[ -e $target ]]; then
  echo "Left $target alone: it is not the plugin's helper"
fi

if [[ -d $state_dir ]]; then
  rm -rf "$state_dir"
  echo "Removed $state_dir"
fi

if ((!keep_udev)) && [[ -e $rule_dst ]]; then
  echo "Removing $rule_dst (needs sudo)"
  sudo rm -f "$rule_dst"
  sudo udevadm control --reload
fi

echo "Done. Remove the plugin with: omarchy plugin remove io.github.z23.flydigi"

# Changelog

## 1.0.0 (2026-11-03)

First release.

- A bar widget and panel for the Flydigi Vader 4 Pro light bar. It offers solid colors, the factory stream effect with speed, off, and brightness, plus an option to follow the theme accent.
- Hotplug comes from udev, with no polling. The last lighting is read from the helper's state file, so the plugin opens the controller only when needed.
- IPC target `io.github.z23.flydigi` for keybindings and scripts.
- `vader4-rgb` 2.0.0 is a dependency-free Go rewrite of the helper from z23/flydigictl:
  - It keeps that tool's commands and adds `--json`, distinct exit codes, `status`, `watch`, `backup`, and `restore`.
  - It no longer unloads the `xpad` module, and it always finishes a write it has started.
- Tested in XInput mode on a Vader 4 Pro (firmware 6.9.3.3) over the 2.4 GHz dongle. DInput mode is covered by golden tests only.

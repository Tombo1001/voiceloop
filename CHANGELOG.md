# Changelog

VoiceLoop's version is in `listener/version.go`. Each release is tagged `v<version>`.

## 0.8.0 (2026-11-09)

The first public release.

- First run writes `config.json` with a random token, so the exe starts without any setup files. Voice mode starts on.
- Settings, About has a new "Pair the hook" card. It shows the addresses the hook can use and the token, and copies a ready-made setup command.
- The hook has `setup`, `test` and `remove` commands. `setup` saves the address and token to `~/.config/voiceloop/hook.json` and adds the hooks to `~/.claude/settings.json`, after a dated backup.
- The hook runs on Windows and macOS as well as Linux, and works without T3 Code. Thread names then fall back to the project folder.
- `scripts/install.ps1` installs VoiceLoop for the current user: autostart, a Start menu entry and, when run as administrator, a firewall rule. `-Uninstall` removes it.
- The pad-profile namespace is now `https://voiceloop.dev/pad-profile/1`. Profiles that use the old `https://github.com/voiceloop/pad-profile/1` still load.
- The exe's version info now matches `version.go`.

## 0.7.0 and earlier

Private builds. Highlights: the on-screen pill, per-thread muting and `/resend`, mute while locked, the Settings panel, macropad set-up and pad profiles, and key symbols exported as STL.

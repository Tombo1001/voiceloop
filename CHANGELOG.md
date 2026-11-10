# Changelog

**voiceloop**'s version is in `listener/version.go`. Each release is tagged `v<version>`.

## 0.8.2 (unreleased)

- Any speech server with an OpenAI-style `/v1/audio/speech` endpoint works, not only Kokoro-FastAPI. Settings, Voice has new Model and API key boxes. The connection test tries `/health`, `/v1/models` and `/v1/audio/voices`, and the voice picker becomes a text box when a server doesn't list its voices. Tested with Kokoro-FastAPI and with Speaches running a Piper voice. See `docs/kokoro.md`.
- Speech audio in any WAV format (stereo, 8 to 32-bit, float, any sample rate) is converted before it plays. Kokoro already sent the one format playback expected.
- The API key stays on the PC: settings exports leave it out, and importing keeps the one already set.
- The pad's Dictate actions follow Handy's own shortcut, read from Handy's `settings_store.json`, instead of assuming Ctrl+Space. If the pad still sends Ctrl+Space after Handy's shortcut changes, Settings, Macropad offers a one-click fix. Key symbols follow too.
- Exported pad profiles carry custom key icons (any that aren't built in) as base64 inside each `<symbol>`, and import saves them to the icons folder. A different picture with the same name is saved as `name-2.png`, so nothing is overwritten. `pad-profile.xsd` allows the icon data, and older profiles still load.
- The name is written "voiceloop", lowercase, in the app, the hook's messages and the docs. Folder names, the Run key, the Start menu shortcut and the firewall rule keep "VoiceLoop", so existing installs carry on as they are.
- The Macropad drawing labels knob turns for pads with more than one knob, beside each knob. Only single-knob pads had them.

## 0.8.1 (2026-11-10)

- The pill now shows emoji in thread names in colour, drawn from Windows' Segoe UI Emoji (its COLR colour layers, gradients included). They used to show as an empty box: a thread called "Second Brain 🧠" read "Second Brain ▯". Emoji joined with a zero-width joiner, such as 👨‍💻, show as their parts, and skin tones show the default colour.

## 0.8.0 (2026-11-09)

The first public release.

- First run writes `config.json` with a random token, so the exe starts without any setup files. Voice mode starts on.
- Settings, About has a new "Pair the hook" card. It shows the addresses the hook can use and the token, and copies a ready-made setup command.
- The hook has `setup`, `test` and `remove` commands. `setup` saves the address and token to `~/.config/voiceloop/hook.json` and adds the hooks to `~/.claude/settings.json`, after a dated backup.
- The hook runs on Windows and macOS as well as Linux, and works without T3 Code. Thread names then fall back to the project folder.
- `scripts/install.ps1` installs **voiceloop** for the current user: autostart, a Start menu entry and, when run as administrator, a firewall rule. `-Uninstall` removes it.
- The pad-profile namespace is now `https://voiceloop.dev/pad-profile/1`. Profiles that use the old `https://github.com/voiceloop/pad-profile/1` still load.
- The exe's version info now matches `version.go`.

## 0.7.0 and earlier

Private builds. Highlights: the on-screen pill, per-thread muting and `/resend`, mute while locked, the Settings panel, macropad set-up and pad profiles, and key symbols exported as STL.

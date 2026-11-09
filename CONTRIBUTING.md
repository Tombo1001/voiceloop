# Contributing

Issues and pull requests are welcome. VoiceLoop is a small project run in spare time, so replies may take a few days.

## Reporting a bug

Say what you expected and what happened. Attach the end of `voiceloop.log` from `%LOCALAPPDATA%\VoiceLoop`, and `hook.log` from `~/.local/state/voiceloop` (or `%LOCALAPPDATA%\VoiceLoop\hook` on Windows) if the hook is involved. Check them first: the hook log holds thread names.

## Asking for another macropad

Use the "My macropad isn't supported" issue template. It asks for the pad's USB vendor and product IDs and, if you've made one, a `pad-profile.xml`. Pads that speak the `ch57x-8890` protocol only need a profile. Other CH57x protocols need an encoder in `listener/macropad.go` first.

## Making changes

- `cd listener && go test ./...` has to pass. It runs on Linux and macOS too, because Windows-only files are build-tagged.
- `python3 -m unittest discover hooks` runs the hook's tests.
- The hook uses the Python standard library only, and must never print anything except a block decision. Claude Code adds hook output to its context.
- The listener is pure Go with no cgo.
- Write British English in UI text, comments and docs.
- Keep each pull request to one change, and say how you tested it. Say so plainly if you couldn't test on Windows.

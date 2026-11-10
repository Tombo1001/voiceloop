# Product

<!-- impeccable:product-schema 1 -->

## Platform

web

## Stack

Plain static HTML, CSS and JavaScript with no build step, deployed to Cloudflare Pages at voiceloop.dev (chosen by Tom, 2026-11-09). The site lives in this repo.

## Users

Developers who run Claude Code (often several sessions at once, often on a headless server or another machine) and want to look away from the screen while an agent works. They sit at a Windows desk PC. Many already dictate their prompts. A secondary audience is people who like desk gadgets and will buy a cheap macropad for one-handed dictate, send and stop.

## Product Purpose

voiceloop reads Claude Code's replies aloud on your Windows PC, even when Claude Code runs on a different machine. Questions and permission prompts get their own chimes, so the user knows when an agent needs them without watching it. Success for the site is a visitor installing it from the GitHub release.

## Positioning

The things voiceloop does that its nearest rival (VoiceMode) doesn't: playback on a different machine from the agent, a native Windows client (tray, on-screen pill, output device choice, mute while locked), per-thread control for people running several agents, and the macropad set up from voiceloop itself. It only speaks. It doesn't listen, and it leaves dictation to tools like Handy. Say plainly what voiceloop does better, and don't run rivals down.

## Operating Context

Three parts: a Python hook on the Claude Code machine, `voiceloop.exe` on the Windows desk PC, and a Kokoro text-to-speech server in Docker on any box (or the same PC). The user picks one of Kokoro's voices in Settings (for example `bf_emma`, `bf_isabella`, `bm_george`, `bm_fable`; the prefix is language and gender, `bf_` British female). Typical moments: an agent finishes a long task while the user is making tea; a permission prompt chimes from another room; `/mute` on a noisy thread; `/resend` to hear the last reply again.

## Capabilities and Constraints

- Real today: speaks replies about 1.3 s after they arrive; three rising notes for a question, ding-dong-ding for an approval; on-screen pill with live bars and the thread name; `/mute`, `/unmute`, `/resend`; only the current thread speaks, or every unmuted thread in turn; silent while the PC is locked; Ctrl+Alt+V voice mode, Ctrl+Alt+X stop; macropad set-up with pad profiles and STL key symbols; first run writes its own token and Settings shows a one-line hook setup command.
- The listener is Windows only (tested on Windows 11). The hook needs Python 3.8+. Kokoro-FastAPI is the only tested speech server.
- Open source, MIT. Free. A paid tier may come later; never imply one exists.
- The release is unsigned, so SmartScreen warns on first run.
- Undecided: demo video, analytics (Cloudflare Web Analytics likely).

## Brand Commitments

- Name: always "voiceloop", lowercase, in the logo and in running text, even at the start of a sentence, and set in bold wherever it appears in body copy (Tom, 2026-11-10). Technical identifiers that already exist (the `%LOCALAPPDATA%\VoiceLoop` folder, the logon entry, firewall rule and scheduled task names) keep their spelling so existing installs still work. The logo is the app's lockup exactly, and Tom wants it kept that way (2026-11-10): the round dark vl badge (`listener/logo/logo.go`: #1d1d1f circle, teal v, #f2f2f4 l, round strokes 11.5% wide) followed by the lowercase wordmark "voiceloop", "voice" in teal #4fd1c5 and "loop" in #ececee, bold, tracking -0.037em (`listener/ui.html` `.logo`). The app sets it in Segoe UI; the web uses a renamed subset of Selawik (Microsoft's OFL stand-in for Segoe UI). On light or busy grounds the lockup sits on the app's dark surface (#1c1c1e) as a pill, never recoloured.
- Colours already in the product: teal `#4fd1c5` (speech, accent), amber `#f6b24e` (question), coral `#ff6b6b` (approval), pill grey `#2a2a2c`. These are commitments of meaning (each colour is an event type), not a full brand kit; the site may go further.
- Voice: the product speaks for itself. The site is in voiceloop's voice, not a maker's, and "pick your computer's voice" is the core idea. The real Kokoro voices are the voice of the site.
- British English everywhere.
- The macropad link is an Amazon affiliate link (`https://link.amazon/B0bSXFqie`) and must carry the disclosure "As an Amazon Associate I earn from qualifying purchases."
- Not affiliated with Anthropic; don't use Anthropic or Claude logos or imply endorsement.

## Evidence on Hand

- `docs/macropad-with-symbols.jpg` (1600 px), `-1200` and `-2000`: Tom's pad with 3D-printed key symbols on a cork mat, cleaned up 2026-11-10. The approved product photo for the site, README and launch posts. It's the only one.
- The Settings panel (`listener/ui.html`) renders in any browser through `tools/ui-mock/mock.py`, so real screenshots of every tab can be captured.
- The pill can be rendered with `listener/render.go` preview tests.
- Real speech: a Kokoro server can synthesise any line in any voice, so audio samples can be real output.
- Starter key icons as SVG in `listener/icons/src/`.
- Absent, do not invent: user counts, stars, testimonials, press, benchmarks beyond the measured 1.3 s start and Kokoro timings in `docs/kokoro.md`, a demo video.

## Product Principles

1. Prove it by speaking. Anything the site says about voice should be heard, not just read.
2. Look away from the screen. The product exists so the user doesn't have to watch; the site should make that feeling obvious.
3. Honest and small. One job done well, with plain limits (Windows only, unsigned, Kokoro-tested).
4. Installing is the goal. Every section should leave the visitor closer to the release download.
5. The pad is a delight, not a requirement.

## Accessibility & Inclusion

Audio must never autoplay with sound; the visitor starts it. Everything spoken must also be readable as text. Respect reduced motion.

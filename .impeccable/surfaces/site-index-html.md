---
version: 1
slug: "site-index-html"
primary_target: "site/index.html"
related_targets: []
---

# voiceloop.dev home page

Scope: the one-page site at `site/index.html`. Mode: Persuade.
Audience: developers running Claude Code (often on another machine) at a Windows desk. Job: understand what voiceloop does, hear it, install it. Action: Download for Windows (GitHub release). Secondary: the pad (affiliate link with disclosure).
Proof on hand: real Kokoro speech in six voices, the real chime notes, the real pill, the cleaned pad photo. No testimonials, stats or prices.
Constraints: name always "voiceloop", lowercase, bold in body copy; the app's lockup unchanged; British English; no autoplay sound.
Memorable moment: the bars and the pill move with the voice you pick.
Unresolved: a demo video. The Download label (hero and Install) names the release and must change with each one.
Related page: `site/pad-profile/1/index.html`, the pad-profile namespace page (Read mode, same world).

## Direction contract

THESIS: The pill's own speech meter, blown up into a loud modular identity that snaps over real photography of the desk. It refuses the dark dev-tool page with a terminal hero.

OWN-WORLD: Cool grey #e6e9ec ground, ink #0a0a0a, and the app's event colours as the only saturated fields: speech teal #4fd1c5, question blue #24a0ff, approval yellow #e8bf2c (Tom's set; the app ships amber and coral as defaults). Long flat bars at -7°, multiplied over photos, small square marks locked to a module. Archivo: 900 at display size, small and businesslike for copy, tracked caps for labels. Hard black blocks for primary actions, outline blocks for secondary. The app's lockup on a dark pill.

STORY: The visitor learns that an agent's reply can be heard at their desk, hears it in a voice they pick, sees how the three parts connect and what each colour means, meets the pad, and downloads.

FIRST VIEWPORT: Left, "Your agent / is done. / Look up." at up to 96px, the one-paragraph pitch, Download (black block) and How it works (outline), then the voice picker. Right, the pad photo at 4:3 with the live pill straddling its bottom edge. Teal, blue and yellow bars cross the headline and the cork above the pad, moving with real speech.

FORM: Dumbar-style modular identity (challenger, studio-dumbar-identity), chosen over the dealt switchboard. seed:8b94fe172f016a58

FINISH: unreviewed and undocumented is unfinished; this build ends with the finish review, the verdict, DESIGN.md, and every shipping raster carrying its provenance

## Finish review, 2026-11-10

Run inline by the build thread (a fork with no subagent tool), against the contract, the craft floor and captures at 1440 and 390 px (`.impeccable/review/desktop.png`, `mobile.png`, not committed). Seed reference corroborated against the original roll output. No approved comp: this is a code-led extension of the incumbent world.

Verdict: ships. Fixed in this pass:
- Bars in the new sections are anchored to their headings (`.hbar`), because percentage positions drifted into body text on phones and floated clear of one-line headings on desktop.
- Commands on phones run the full width under their number tab instead of breaking mid-flag. Inline code no longer wraps.
- Keyboard focus was invisible on ink surfaces (footer, notes). It's teal there now.
- The How it works location labels sat above their headings as eyebrows (a craft-floor ban). They now sit under the heading.
- The outline button collapsed to text height when buttons wrapped. It's at least 52px tall.

Left as they are: the hero Download label wraps to two lines at 390 px (readable, approved earlier); the top nav has no Install link (Tom approved the three-item nav; the footer links to Install).

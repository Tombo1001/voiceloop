---
name: voiceloop.dev
description: The pill's speech meter, blown up into a loud modular identity over real desk photography.
colors:
  ground-grey: "#e6e9ec"
  seam-grey: "#d5d9de"
  ink: "#0a0a0a"
  ink-soft: "#3d4045"
  paper: "#ffffff"
  speech-teal: "#4fd1c5"
  question-blue: "#24a0ff"
  approval-yellow: "#e8bf2c"
  plate: "#1c1c1e"
  badge: "#1d1d1f"
  loop-grey: "#ececee"
  pill-grey: "#2a2a2c"
  on-ink-soft: "#c9cdd1"
  code-on-ink: "#26262a"
  idle-grey: "#6a6a70"
typography:
  display:
    fontFamily: "Archivo, system-ui, sans-serif"
    fontSize: "clamp(56px, 6.7vw, 96px)"
    fontWeight: 900
    lineHeight: 0.9
    letterSpacing: "-0.03em"
    fontVariation: "'wdth' 88"
  headline:
    fontFamily: "Archivo, system-ui, sans-serif"
    fontSize: "clamp(44px, 5.4vw, 80px)"
    fontWeight: 900
    lineHeight: 0.92
    letterSpacing: "-0.03em"
    fontVariation: "'wdth' 88"
  title:
    fontFamily: "Archivo, system-ui, sans-serif"
    fontSize: "22px"
    fontWeight: 800
    lineHeight: 1.15
    letterSpacing: "-0.01em"
  lead:
    fontFamily: "Archivo, system-ui, sans-serif"
    fontSize: "19px"
    fontWeight: 400
    lineHeight: 1.45
  body:
    fontFamily: "Archivo, system-ui, sans-serif"
    fontSize: "17px"
    fontWeight: 400
    lineHeight: 1.5
  label:
    fontFamily: "Archivo, system-ui, sans-serif"
    fontSize: "13px"
    fontWeight: 800
    letterSpacing: "0.1em"
  button:
    fontFamily: "Archivo, system-ui, sans-serif"
    fontSize: "16px"
    fontWeight: 800
    lineHeight: 1.1
    letterSpacing: "0.08em"
  mono:
    fontFamily: "ui-monospace, Cascadia Mono, SF Mono, Consolas, monospace"
    fontSize: "0.92em"
    fontWeight: 600
  wordmark:
    fontFamily: "VL Wordmark, Segoe UI, sans-serif"
    fontWeight: 700
    letterSpacing: "-0.037em"
  pill:
    fontFamily: "VL Pill, Segoe UI, system-ui, sans-serif"
    fontSize: "14.5px"
    fontWeight: 600
rounded:
  block: "0px"
  overlay: "24px"
  capsule: "999px"
spacing:
  seam: "2px"
  gutter: "clamp(16px, 3.3vw, 48px)"
  section: "clamp(72px, 9vw, 140px)"
  max: "1680px"
components:
  button-primary:
    backgroundColor: "{colors.ink}"
    textColor: "{colors.paper}"
    typography: "{typography.button}"
    rounded: "{rounded.block}"
    padding: "18px 24px"
  button-primary-hover:
    backgroundColor: "{colors.speech-teal}"
    textColor: "{colors.ink}"
  button-ghost:
    backgroundColor: "{colors.ground-grey}"
    textColor: "{colors.ink}"
    rounded: "{rounded.block}"
    padding: "0 18px"
    height: "52px"
  button-ghost-hover:
    backgroundColor: "{colors.ink}"
    textColor: "{colors.paper}"
  nav-link-hover:
    backgroundColor: "{colors.plate}"
    textColor: "{colors.speech-teal}"
    rounded: "{rounded.capsule}"
    padding: "10px 18px"
  lockup-plate:
    backgroundColor: "{colors.plate}"
    textColor: "{colors.speech-teal}"
    typography: "{typography.wordmark}"
    rounded: "{rounded.capsule}"
  code-chip:
    backgroundColor: "{colors.ink}"
    textColor: "{colors.paper}"
    typography: "{typography.mono}"
    rounded: "{rounded.block}"
    padding: "5px 8px"
  number-tab:
    backgroundColor: "{colors.ink}"
    textColor: "{colors.paper}"
    rounded: "{rounded.block}"
    width: "36px"
  speech-pill:
    backgroundColor: "{colors.pill-grey}"
    textColor: "{colors.loop-grey}"
    typography: "{typography.pill}"
    rounded: "{rounded.overlay}"
    height: "86px"
---

# Design System: voiceloop.dev

Recorded on 2026-11-10 from the shipped `site/index.html`, `site/pad-profile/1/index.html` and `site/assets/site.css`. The tokens above are what the CSS uses. When this file and the CSS disagree, the CSS wins and this file needs updating.

## Overview

**North star: the speech meter, blown up.** The pill that voiceloop shows while it talks has a row of small bars that move with the voice. The site takes those bars, makes them huge and flat, and lays them across the headings and the photo of the pad at -7°. Everything else is a modular grid of hard blocks on a cool grey ground. The look is loud, flat and Swiss, closer to a studio identity than a developer-tool page. It is a light page because people look at it at a desk in daylight, and because the dark terminal hero is what every rival does.

The page proves the product by sound. The bars in the hero and the pills move with real Kokoro speech when the visitor presses a voice. Nothing plays until they do.

Key characteristics:
- Cool grey ground, ink type and blocks, white panels. Three saturated colours and no others.
- Flat colour bars at -7°, multiplied over what they cross.
- Tiles separated by 2px seams of the parent colour, never by borders or shadows.
- Archivo at 900 and 88% width for headings. Small, plain copy.
- The app's own lockup and pill, reproduced exactly.

## Colors

### Primary
- **Ink** (`#0a0a0a`): type, primary buttons, number tabs, code chips, the dark panels (the app node, "voiceloop suits you if", the footer).

### Event colours
These three are the only saturated colours, and each means what it means in the app's pill:
- **Speech Teal** (`#4fd1c5`): speech and voiceloop itself. The "voice" half of the wordmark, the button-hover arrow cell, active pins and keys, and the speech bar.
- **Question Blue** (`#24a0ff`): questions. The How it works nav hover, the question demo and the Compared with VoiceMode bar.
- **Approval Yellow** (`#e8bf2c`): approvals. The pad nav hover, the approval demo, and the pad, Security and pad-profile bars.

The app ships amber `#f6b24e` and coral `#ff6b6b` as its own defaults for question and approval. The site uses Tom's blue and yellow instead (2026-11-10), and the app's defaults stay as they are.

### Neutral
- **Ground Grey** (`#e6e9ec`): the page ground and alternate sections.
- **Paper** (`#ffffff`): alternate sections and panels on grey.
- **Seam Grey** (`#d5d9de`): the How it works wire and hover fills.
- **Ink Soft** (`#3d4045`): secondary text on light grounds (7:1 or better on grey and white).
- **On-ink Soft** (`#c9cdd1`): secondary text on ink.
- **Plate** (`#1c1c1e`), **Badge** (`#1d1d1f`), **Loop Grey** (`#ececee`), **Pill Grey** (`#2a2a2c` at 95%): the app's own surfaces, used only for the lockup, the nav hover pill and the speech pill.

### Named rules
**Three colours, three meanings.** Teal, blue and yellow are the only saturated colours on the site. Teal leads. Never add a fourth hue, and never use a colour for something that contradicts its meaning, such as yellow on a speech element.

**One bar per heading.** Each section heading gets exactly one bar. Two multiplied bars crossing behind a heading make a near-black band that hurts reading (Tom flagged it on 2026-11-10).

## Typography

Archivo, self-hosted as a variable subset, is the only text face. The wordmark and the pill use renamed subsets of Microsoft's Selawik (SIL OFL 1.1; Selawik is a Reserved Font Name, hence "VL Wordmark" and "VL Pill") to match the app's Segoe UI.

### Hierarchy
- **Display** (900, clamp 56 to 96px, line-height 0.9, -0.03em, 88% width): the hero headline only, one line per span.
- **Headline** (900, clamp 44 to 80px, 0.92): section headings. The pad-profile page's h1 uses the same treatment at up to 88px.
- **Title** (800, 22px): sub-sections, panel and step headings. In the "Hear each kind" and "Without leaving the keyboard" rows it scales to 26 to 34px at 900.
- **Lead** (19px, 1.45): the paragraph beside each section heading. About 36 to 46ch.
- **Body** (17px, 1.5): everything else. Measure capped at 52 to 68ch.
- **Label** (800, 13px, 0.1em, uppercase): small labels inside a component, such as the location under a How it works heading, the security row names and "Pick its voice".
- **Mono** (600 or 500, ui-monospace stack): only for commands, file names, ports, keys and the namespace URL.

### Named rules
**Lowercase voiceloop.** The name is always "voiceloop", lowercase, even at the start of a sentence, and bold in running text. Technical identifiers that already exist (`%LOCALAPPDATA%\VoiceLoop`, the task and firewall rule names) keep their spelling.

**Mono is for things you type.** Monospace marks real commands, paths, ports and keys. It's never used to make prose look technical.

## Layout

- One centred column, `max-width: 1680px`, with a side gutter of `clamp(16px, 3.3vw, 48px)`.
- Sections alternate grey and white, with top and bottom padding of `clamp(72px, 9vw, 140px)`.
- A section opens with a two-column head: the heading on the left, the lead paragraph on the right, aligned to the bottom.
- Grids are tiles on a 2px seam: the container takes the parent's colour and its children sit 2px apart (the voice picker, the kinds, the commands, the key list, the comparison, the security rows, the install steps, the pad-profile steps).
- The hero is one box of two equal columns. Its bars are positioned in percentages of that box, so they stay locked to the photo at any size.
- One breakpoint, 900px: below it every grid becomes a single column, the photo moves above the hero copy, and install commands run the full width under their number tab. At 390px nothing scrolls sideways.

## Elevation & Depth

The page is flat. Depth comes from tonal layers (ink on grey on white) and from bars multiplied over what's beneath them. Only two things cast shadows, and both are objects from the real world:
- The speech pill: `0 14px 30px rgba(0,0,0,.32)`, as it floats over the desktop in the app.
- The numbered pins on the pad photo: `0 6px 14px rgba(0,0,0,.3)`.

An active or lit tile shows an 8px inset bar along its bottom edge in its event colour (the How it works nodes while a demo runs, the pad-profile file links).

## Shapes

- Blocks are square-cornered: buttons, panels, tiles, chips, number tabs and code.
- Only the app's own objects are round: the lockup plate and nav hover (capsule), the speech pill (24px) with its round dot and buttons, and the vl badge.
- Bars are long flat rectangles at -7°, `mix-blend-mode: multiply`. Small square marks (10px) mark list items.

## Components

### Buttons
- **Primary** (`.do`): a hard ink block with an uppercase label, a smaller sentence-case line under it (version, store), and a separate arrow cell divided by a faint rule. On hover the arrow cell turns teal and the arrow ink.
- **Ghost** (`.ghost`): a 2px ink outline on the ground colour, uppercase, at least 52px tall. It fills ink on hover.

### Navigation
Uppercase links with a 16px icon. On hover or focus each one becomes the lockup's dark capsule with text in its own event colour (`--hc`): blue for How it works, yellow for the pad, teal for GitHub.

### The lockup
The app's lockup exactly: the dark vl badge, then "voice" in teal and "loop" in loop grey, bold, -0.037em. On light or busy grounds it sits on the plate capsule. It is never recoloured.

### The speech pill
A port of the Settings preview (`listener/ui.html` `.pill.two`): a dot, the thread name, a Mute thread chip, a ✕ button and nine bars below, driven by the audio's loudness. Idle or muted, the dot and bars go grey (`#6a6a70`).

### Tiles and rows
- **Number tab:** an ink square with a white numeral at the start of a key, step or pin.
- **Code chip:** ink with white mono text, for ports and commands in labels. Inline code in copy sits on the opposite neutral (grey on white, white on grey) with a 1 to 5px pad and doesn't wrap.
- **Command block:** an ink block with pre-wrapped mono at 15px (13px on phones).
- **Comparison:** their case on grey, ours on ink, each item marked with a 10px square (grey for theirs, teal for ours).

### Heading bars
The older sections place their bar in `.sec-bars` by percentage. The newer sections (and the pad-profile page) put a `.hbar` inside the heading, so the bar crosses the heading's first line at every width and still starts off the page's left edge.

## Do's and Don'ts

### Do:
- Make the page speak. Anything about voice should be playable, never on autoplay, and always readable as text too.
- Use teal, blue and yellow only with their meanings, and keep the rest ink, grey and white.
- Put one bar on each new section heading, in the colour that fits the section.
- Build groups as tiles on a 2px seam.
- Show the real thing: the pad photo, the real pill, real Kokoro output, real commands.
- Keep focus visible: the ring is ink on light grounds and teal on ink surfaces.

### Don't:
- Don't add a fourth saturated colour, gradients, glass or glow.
- Don't put a tracked-caps label above a heading. Labels go under or beside the heading.
- Don't cross two bars behind text.
- Don't round the blocks, or add borders or drop shadows to tiles.
- Don't recolour or re-letter the lockup, and don't write "VoiceLoop" in copy.
- Don't use monospace for prose, or invent numbers, users or quotes.

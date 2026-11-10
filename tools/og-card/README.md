# Social card

`card.html` is the source of `site/assets/img/og.jpg`, the 1200×630 image link previews show for voiceloop.dev.

It uses the site's own fonts and `site/assets/img/pad-1200.jpg` (Tom's pad photo), so serve the repo root and open it at exactly 1200×630:

```bash
python3 -m http.server 8806 --directory .          # from the repo root
# open http://localhost:8806/tools/og-card/card.html in a 1200x630 viewport and screenshot it
ffmpeg -i card.png -q:v 3 site/assets/img/og.jpg   # about 125 KB
```

Provenance: rendered 2026-11-10 in Chromium from this file. The only photograph in it is Tom's pad photo. The pill, bars and lettering are drawn in HTML and CSS.

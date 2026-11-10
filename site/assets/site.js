// voiceloop.dev: real Kokoro speech, the app's own chimes, and everything that moves with them.
"use strict";

const VOICES = [
  ["bf_emma", "Emma", "British"], ["bm_george", "George", "British"],
  ["bf_isabella", "Isabella", "British"], ["bm_fable", "Fable", "British"],
  ["af_heart", "Heart", "American"], ["am_michael", "Michael", "American"],
];
const still = matchMedia("(prefers-reduced-motion: reduce)").matches;
let voice = "bf_emma";

// ---- Sound ----
let ctx, el, analyser, owner = null;
function audio() {
  if (!ctx) ctx = new (window.AudioContext || window.webkitAudioContext)();
  if (ctx.state === "suspended") ctx.resume();
  return ctx;
}
// The notes and bell envelope from listener/tones.go.
const CHIMES = {
  question: [[659.3, 0, .45, 7], [784.0, .12, .45, 7], [1046.5, .24, .7, 5]],
  approval: [[880, 0, .3, 9], [659.3, .16, .3, 9], [880, .32, .3, 9], [880, .62, .3, 9], [659.3, .78, .3, 9], [880, .94, .5, 6]],
};
function chime(kind) {
  const a = audio(), t0 = a.currentTime + 0.02, out = a.createGain();
  out.gain.value = 0.16; out.connect(a.destination);
  for (const [f, s, d, dec] of CHIMES[kind]) {
    for (const [h, amp] of [[1, 1], [2, .35], [3, .12]]) {
      const o = a.createOscillator(), g = a.createGain();
      o.frequency.value = f * h;
      g.gain.setValueAtTime(0, t0 + s);
      g.gain.linearRampToValueAtTime(amp, t0 + s + 0.004);
      g.gain.setTargetAtTime(0, t0 + s + 0.004, 1 / dec);
      o.connect(g); g.connect(out);
      o.start(t0 + s); o.stop(t0 + s + d + 0.05);
    }
  }
  return (CHIMES[kind].at(-1)[1] + 0.5) * 1000;
}
function speak(line, who, onend) {
  const a = audio();
  if (!el) {
    el = new Audio();
    const src = a.createMediaElementSource(el);
    analyser = a.createAnalyser(); analyser.fftSize = 512; analyser.smoothingTimeConstant = 0.6;
    src.connect(analyser); analyser.connect(a.destination);
  }
  el.pause(); el.src = `assets/audio/${line}-${voice}.mp3`; el.currentTime = 0;
  owner = who;
  el.onended = () => { owner = null; onend && onend(); };
  return el.play().catch(() => { owner = null; });
}
function stopSpeech() { if (el) el.pause(); owner = null; }
function speaking(who) { return !!el && !el.paused && !el.ended && (!who || owner === who); }
function level() {
  if (!speaking()) return 0;
  const d = new Uint8Array(analyser.fftSize); analyser.getByteTimeDomainData(d);
  let s = 0; for (const v of d) { const x = (v - 128) / 128; s += x * x; }
  return Math.min(1, Math.sqrt(s / d.length) * 3.2);
}
function bands(n) {
  const out = new Array(n).fill(0);
  if (!speaking()) return out;
  const d = new Uint8Array(analyser.frequencyBinCount); analyser.getByteFrequencyData(d);
  const per = Math.floor(d.length * 0.5 / n);
  for (let i = 0; i < n; i++) { let m = 0; for (let j = 0; j < per; j++) m = Math.max(m, d[i * per + j]); out[i] = m / 255; }
  return out;
}

// ---- Hero bars: the pill's level meter, blown up. [colour, top %, thickness %, width %, rotation] ----
const SPEC = [
  ["var(--teal)", 3, 13, 62, -7], ["var(--question)", 18, 10, 64, -7], ["var(--approval)", 33, 12, 86, -7],
  ["var(--teal)", 62, 8, 50, -7], ["var(--ink)", 101, 3.2, 30, -7],
];
const MARKS = [["var(--approval)", 8, 62, 44, 18, 24], ["var(--question)", 88, 18, 26, 26, -18], ["var(--ink)", 92, 72, 18, 18, 30], ["var(--teal)", 40, 88, 60, 14, -10], ["var(--ink)", 58, 12, 14, 14, 12]];
const barLayer = document.getElementById("bars");
const bars = SPEC.map(([c, t, h, w, r]) => { const b = document.createElement("i"); b.className = "bar";
  b.style.cssText = `--c:${c};--t:${t}%;--h:${h}%;--w:${w}%;--r:${r}deg;--s:1`; barLayer.append(b); return b; });
const marks = MARKS.map(([c, x, y, w, h, r]) => { const d = document.createElement("i"); d.className = "mark";
  d.style.cssText = `--c:${c};left:${x}%;top:${y}%;--w:${w}px;--h:${h}px;--r:${r}deg`; barLayer.append(d); return d; });
let barsRaf;
function moveBars() {
  const b = bands(5), on = speaking();
  bars.forEach((el, i) => el.style.setProperty("--s", on ? (0.28 + b[i] * 0.95).toFixed(3) : 1));
  marks.forEach((el, i) => el.style.transform = on ? `rotate(0deg) translate(${(b[i] * 16).toFixed(1)}px, 0)` : "");
  if (on) barsRaf = requestAnimationFrame(moveBars);
}
function startBars() { if (still) return; cancelAnimationFrame(barsRaf); barsRaf = requestAnimationFrame(moveBars); }

// ---- Voice picker ----
const list = document.getElementById("voice-list"), swatch = ["var(--teal)", "var(--question)", "var(--approval)"];
VOICES.forEach(([id, name, accent], i) => {
  const b = document.createElement("button");
  b.type = "button";
  b.innerHTML = `<i style="--c:${swatch[i % 3]}"></i>${name}<small>${accent}</small>`;
  b.setAttribute("aria-pressed", "false");
  b.setAttribute("aria-label", `Hear ${name}, ${accent}`);
  b.onclick = () => {
    voice = id;
    list.querySelectorAll("button").forEach(x => x.setAttribute("aria-pressed", x === b));
    speak("reply", "hero", () => b.setAttribute("aria-pressed", "false"));
    startBars();
  };
  list.append(b);
});

// ---- Pills: the Settings preview's idle animation, or the real speech when it's theirs ----
const pills = [...document.querySelectorAll(".vlpill")].map(p => ({
  el: p, bars: [...p.querySelectorAll(".pbars i")], who: p.dataset.who, idle: p.dataset.idle !== "off", muted: false, loud: 0,
}));
const t0 = performance.now();
(function tick(now) {
  const t = (now - t0) / 1000, lv = level();
  for (const p of pills) {
    const mine = speaking(p.who) || (p.who === "hero" && speaking());
    p.loud = p.loud * .6 + (mine ? lv : 0) * .4;
    p.bars.forEach((el, i) => {
      const d = Math.abs(i - 4);
      let v;
      if (p.muted) v = 0;
      else if (mine) v = Math.min(1, p.loud * 3.2 * (.62 + .38 * Math.sin(t * 11 + i * 1.7)) * (1 - .1 * d));
      else if (!p.idle) v = 0;
      else if (still) v = .55 - .1 * d;
      else v = Math.max(0, Math.sin(t * 5.3 - d * .6) * .5 + Math.sin(t * 8.1 + i) * .3 + .25) * (1 - .13 * d);
      el.style.height = (4.5 + Math.max(0, v) * 29.5) + "px";
    });
  }
  requestAnimationFrame(tick);
})(t0);
for (const p of pills) {
  const name = p.el.querySelector(".name"), label = name.textContent;
  p.el.querySelector(".x").onclick = () => stopSpeech();
  p.el.querySelector(".mute").onclick = e => {
    p.muted = !p.muted; p.el.classList.toggle("muted", p.muted);
    e.currentTarget.textContent = p.muted ? "Unmute" : "Mute thread";
    if (p.who === "hero") name.textContent = p.muted ? "voiceloop is muted" : label;
    if (p.muted) stopSpeech();
  };
}

// ---- How it works: send a reply, a question or an approval along the chain ----
const chain = document.querySelector(".chain"), nodes = [...chain.querySelectorAll(".node")], packet = chain.querySelector(".packet");
const howPill = pills.find(p => p.who === "how");
const KINDS = {
  reply: { colour: "var(--speech)", thread: "fix the login bug" },
  question: { colour: "var(--question)", thread: "refactor billing" },
  approval: { colour: "var(--approval)", thread: "deploy to staging" },
};
function centre(n) {
  const a = n.getBoundingClientRect(), c = chain.getBoundingClientRect(), vertical = getComputedStyle(chain).gridTemplateColumns.split(" ").length < 2;
  return vertical ? { x: 29, y: a.top - c.top + a.height / 2 } : { x: a.left - c.left + a.width / 2, y: c.height / 2 };
}
function hop(from, to, ms) {
  const a = centre(from), b = centre(to);
  if (still) { packet.style.transform = `translate(${b.x}px, ${b.y}px)`; return Promise.resolve(); }
  return packet.animate([{ transform: `translate(${a.x}px, ${a.y}px)` }, { transform: `translate(${b.x}px, ${b.y}px)` }],
    { duration: ms, easing: "cubic-bezier(.5, 0, .2, 1)", fill: "forwards" }).finished;
}
function light(n, colour) { n.style.setProperty("--c", colour); n.classList.add("lit"); setTimeout(() => n.classList.remove("lit"), 700); }
let run = 0;
document.querySelectorAll(".kinds button").forEach(btn => btn.addEventListener("click", async () => {
  const kind = btn.dataset.kind, k = KINDS[kind], me = ++run;
  document.querySelectorAll(".kinds button").forEach(x => x.setAttribute("aria-pressed", x === btn));
  stopSpeech();
  audio(); // unlock sound inside the click
  howPill.el.style.setProperty("--pc", k.colour);
  howPill.el.querySelector(".name").textContent = k.thread;
  howPill.el.classList.remove("idle");
  packet.style.setProperty("--c", k.colour); packet.style.opacity = 1;
  const [hook, app, kokoro] = nodes;
  light(hook, k.colour);
  await hop(hook, app, 520); if (me !== run) return;
  light(app, k.colour);
  let wait = 0;
  if (kind !== "reply") wait = chime(kind);
  await hop(app, kokoro, 420); if (me !== run) return;
  light(kokoro, k.colour);
  await hop(kokoro, app, 420); if (me !== run) return;
  packet.style.opacity = 0;
  await new Promise(r => setTimeout(r, Math.max(0, wait - 840)));
  if (me !== run) return;
  speak(kind, "how", () => { if (me === run) { btn.setAttribute("aria-pressed", "false"); howPill.el.classList.add("idle"); } });
  startBars();
}));

// ---- The pad: point at a control and its pin lights up ----
const pins = document.querySelectorAll(".pin");
document.querySelectorAll(".keys li").forEach(li => {
  const on = v => { li.classList.toggle("on", v); pins.forEach(p => p.classList.toggle("on", v && p.dataset.k === li.dataset.k)); };
  li.addEventListener("mouseenter", () => on(true)); li.addEventListener("mouseleave", () => on(false));
  li.addEventListener("focusin", () => on(true)); li.addEventListener("focusout", () => on(false));
});

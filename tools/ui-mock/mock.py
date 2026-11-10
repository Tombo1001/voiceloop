"""Serve ui.html with a fake voiceloop API, to check the Macropad tab renders.
Usage: python3 mock.py UI_HTML PORT [big]"""
import json, sys
from http.server import BaseHTTPRequestHandler, HTTPServer

UI, PORT, BIG = sys.argv[1], int(sys.argv[2]), len(sys.argv) > 3

def profile():
    p = {"version": 1, "name": "3 keys + knob (CH57x)", "protocol": "ch57x-8890",
         "usb": {"vendor_id": "0x1189", "product_id": "0x8890", "usage_page": "0xFF00", "report_length": 65},
         "limits": {"max_sequence": 5, "modifiers_first_only": True},
         "keys": {"rows": 1, "columns": 3, "list": [{"id": f"key{i}", "code": i} for i in (1, 2, 3)]},
         "knobs": [{"id": "knob1", "turn_left": 13, "press": 14, "turn_right": 15}],
         "lights": [{"value": 0, "name": "Off"}, {"value": 1, "name": "Pulsing, changes colour on each key press"}, {"value": 2, "name": "Carousel, lights each key in turn"}],
         "mapping": {"knob_side": "right", "lights": 0, "bind": [
             {"control": "key1", "action": "ctrl-space"}, {"control": "key2", "action": "ctrl-space,slash"},
             {"control": "key3", "action": "enter"}, {"control": "knob1.left", "action": "volumedown"},
             {"control": "knob1.press", "action": "ctrl-alt-x"}, {"control": "knob1.right", "action": "volumeup"}]},
         "symbols": {"shape": "square", "size": 9, "layers": 2, "layer_height": 0.2, "list": [
             {"control": "key1", "icon": "dictate.png"}, {"control": "key2", "icon": "slash-command.png"},
             {"control": "key3", "icon": "send.png"}, {"control": "knob1", "icon": "stop-speech.png"}]}}
    if BIG:
        p["name"] = "Example 6 keys + 2 knobs"
        p["keys"] = {"rows": 2, "columns": 3, "list": [{"id": f"key{i}", "code": i} for i in range(1, 7)]}
        p["knobs"] = [{"id": "knob1", "turn_left": 13, "press": 14, "turn_right": 15}, {"id": "knob2", "turn_left": 16, "press": 17, "turn_right": 18}]
        p["mapping"]["bind"] = p["mapping"]["bind"][:3] + [{"control": "key4", "action": "ctrl-alt-v"}, {"control": "key6", "action": "f13"}, {"control": "knob2.press", "action": "mute"}]
        p["lights"] = []
    return p

STATE = {"settings": {"voice": "bf_emma", "speed": 1, "anchor": "bottom-centre", "thread_mode": "all", "pill_size": 100,
                      "speech_colour": "#2cd888", "question_colour": "#24a0ff", "approval_colour": "#e8bf2c", "pill_colour": "#2a2a2c",
                      "pill_opacity": 95, "max_title": 260, "thread_trim_hours": 24, "kokoro_url": "http://example:8880", "tts_model": "kokoro", "tts_api_key": "", "speech_char_limit": 600},
         "monitors": [{"name": "Display 1", "device": "d1", "primary": True, "width": 2560, "height": 1440, "scale": 1.25}],
         "devices": ["Speakers"], "voice_on": True, "threads": [], "version": "0.7.0-mock", "folder": "C:\\Users\\you\\AppData\\Local\\VoiceLoop",
         "pairing": {"token": "0123456789abcdef0123456789abcdef0123456789abcdef", "urls": ["http://10.0.0.20:8765", "http://100.64.0.7:8765"]}}

class H(BaseHTTPRequestHandler):
    def log_message(self, *a): pass
    def send(self, obj, ctype="application/json"):
        b = obj if isinstance(obj, bytes) else json.dumps(obj).encode()
        self.send_response(200); self.send_header("Content-Type", ctype); self.end_headers(); self.wfile.write(b)
    def do_GET(self):
        if self.path.startswith("/api/events"):
            self.send_response(200); self.send_header("Content-Type", "text/event-stream"); self.end_headers()
            self.wfile.write(b"event: hello\ndata: mock\n\n"); self.wfile.flush()
            import time; time.sleep(600); return
        if self.path.startswith("/api/state"): return self.send(STATE)
        if self.path.startswith("/api/pad/symbol.png"):
            import urllib.parse, os
            c = urllib.parse.parse_qs(urllib.parse.urlparse(self.path).query)["control"][0]
            f = os.path.join(os.path.dirname(__file__), "sym", c + ".png")
            if os.path.exists(f): return self.send(open(f, "rb").read(), "image/png")
            self.send_response(404); self.end_headers(); return
        if self.path.startswith("/api/pad"): return self.send({"profile": profile(), "status": {"found": True, "model": "mock"}})
        if self.path.startswith("/?") or self.path == "/": return self.send(open(UI, "rb").read(), "text/html; charset=utf-8")
        self.send_response(404); self.end_headers()
    def do_POST(self):
        self.rfile.read(int(self.headers.get("Content-Length", 0)))
        if self.path.startswith("/api/test-speech"): return self.send({"ok": True, "ms": 12, "voices": ["bf_emma"]})
        if self.path.startswith("/api/pad"): return self.send({"ok": True, "profile": profile(), "status": {"found": True}})
        return self.send({"ok": True})

from socketserver import ThreadingMixIn
class TS(ThreadingMixIn, HTTPServer): daemon_threads = True
TS(("0.0.0.0", PORT), H).serve_forever()

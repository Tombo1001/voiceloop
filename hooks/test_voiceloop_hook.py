"""Tests for voiceloop_hook.py. Run: python3 -m unittest discover hooks"""
import json
import os
import sys
import tempfile
import unittest

sys.path.insert(0, os.path.dirname(os.path.abspath(__file__)))
import voiceloop_hook as h  # noqa: E402


def read_json(p):
    with open(p) as f:
        return json.load(f)


class Speakable(unittest.TestCase):
    def test_strips_markdown(self):
        md = "## Done\n\nI fixed **the bug** in `~/code/voiceloop`. See [the docs](https://x.y).\n\n```go\nx := 1\n```\n| a | b |\n|---|---|\n- one\n- two"
        self.assertEqual(h.speakable(md), "Done. I fixed the bug in voiceloop. See the docs. one. two.")

    def test_slash_commands_and_links(self):
        self.assertEqual(h.speakable("Type /resend or `/mute`, then open https://example.com"),
                         "Type slash resend or slash mute, then open a link.")

    def test_long_code_span(self):
        self.assertEqual(h.speakable("Run `" + "x " * 40 + "` now."), "Run a command now.")

    def test_never_ends_on_lead_in(self):
        old = h.MAX_CHARS
        h.MAX_CHARS = 40
        try:
            self.assertEqual(h.speakable("First point here. Two things to know:\n- a long item that will not fit"),
                             "First point here.")
        finally:
            h.MAX_CHARS = old


class Commands(unittest.TestCase):
    def test_mute(self):
        for t in ["/mute", "slash mute", "voiceloop unmute", "/mute this thread."]:
            self.assertTrue(h.MUTE_RE.match(t), t)
        for t in ["mute the tv", "/muted", "please /mute"]:
            self.assertFalse(h.MUTE_RE.match(t), t)

    def test_resend(self):
        for t in ["/resend", "/repeat", "slash replay", "/resend the last reply"]:
            self.assertTrue(h.RESEND_RE.match(t), t)
        self.assertFalse(h.RESEND_RE.match("resend the email"))

    def test_question_text(self):
        q = {"questions": [{"question": "Which one?", "options": [{"label": "A"}, {"label": "B"}, {"label": "C"}]}]}
        self.assertEqual(h.question_text(q), "Question. Which one? Options: A, B or C.")


class Settings(unittest.TestCase):
    def test_setup_and_remove_keep_other_hooks(self):
        with tempfile.TemporaryDirectory() as d:
            p = os.path.join(d, "settings.json")
            other = {"hooks": [{"type": "command", "command": "echo other"}]}
            with open(p, "w") as f:
                json.dump({"model": "x", "hooks": {"Stop": [other]}}, f)
            h.edit_settings(p, add=True)
            h.edit_settings(p, add=True)  # running setup twice mustn't duplicate
            s = read_json(p)
            self.assertEqual(len(s["hooks"]["Stop"]), 2)
            self.assertEqual(s["hooks"]["PreToolUse"][0]["matcher"], "AskUserQuestion")
            self.assertTrue(all(h.is_ours(e) for e in s["hooks"]["UserPromptSubmit"]))
            h.edit_settings(p, add=False)
            self.assertEqual(read_json(p), {"model": "x", "hooks": {"Stop": [other]}})
            self.assertEqual(len([n for n in os.listdir(d) if ".bak-voiceloop-" in n]), 3)

    def test_env_wins_over_file(self):
        with tempfile.TemporaryDirectory() as d:
            conf = os.path.join(d, "hook.json")
            with open(conf, "w") as f:
                json.dump({"url": "http://file:1/", "token": "filetok"}, f)
            old_conf, old_env = h.CONFIG_FILE, dict(os.environ)
            h.CONFIG_FILE = conf
            try:
                os.environ.pop("VOICELOOP_URL", None)
                os.environ.pop("VOICELOOP_TOKEN", None)
                self.assertEqual(h.load_config(), ("http://file:1", "filetok"))
                os.environ["VOICELOOP_URL"] = "http://env:2"
                self.assertEqual(h.load_config(), ("http://env:2", "filetok"))
            finally:
                h.CONFIG_FILE = old_conf
                os.environ.clear()
                os.environ.update(old_env)


if __name__ == "__main__":
    unittest.main()

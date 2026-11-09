#!/usr/bin/env python3
"""Claude Code hook for VoiceLoop. Python 3.8+, standard library only.

Setting up (run once, on the machine where Claude Code runs):

  python3 voiceloop_hook.py setup --url http://PC-ADDRESS:8765 --token TOKEN
  python3 voiceloop_hook.py test     sends a test phrase to the VoiceLoop PC
  python3 voiceloop_hook.py remove   takes the hooks out of ~/.claude/settings.json

Settings, About in VoiceLoop shows the address and token, and copies the setup command.

The modes Claude Code runs:

  voiceloop_hook.py prompt   (UserPromptSubmit) remembers which session you last typed into
  voiceloop_hook.py stop     (Stop) speaks that session's final reply on the VoiceLoop PC
  voiceloop_hook.py ask      (PreToolUse, AskUserQuestion) question chime, then reads the question
  voiceloop_hook.py notify   (Notification) approval chime for permission prompts

Typing /mute or /unmute in a thread (or dictating "slash mute") is caught in prompt mode:
the listener's mute list is updated and the prompt is blocked, so Claude never sees it.
/resend (also /repeat, /replay) reads the thread's last reply out again, the same way.

Every reply is sent with its thread id and title (from T3 Code when it's
installed, otherwise the Claude session id and project folder), and whether
it's the thread you last typed into. The listener applies the thread mode (current
thread only, or all unmuted threads), the mute list and voice mode.

The hook must never block Claude or print, except for the mute command's
block decision: UserPromptSubmit stdout is added to Claude's context.
Network calls for replies run in a forked child where the OS allows it.

The address and token come from VOICELOOP_URL and VOICELOOP_TOKEN if set,
otherwise from hook.json in the config folder (see config_dir()).
"""
import json
import os
import re
import sys
import time
import urllib.error
import urllib.request

MAX_CHARS = 20000  # sanity cap only: the listener applies your limit (Settings, Voice)
HOOK_MODES = ("prompt", "stop", "ask", "notify")


def config_dir():
    """Where hook.json lives: %APPDATA%\\VoiceLoop on Windows, else $XDG_CONFIG_HOME/voiceloop."""
    if os.name == "nt":
        return os.path.join(os.environ.get("APPDATA") or os.path.expanduser("~"), "VoiceLoop")
    return os.path.join(os.environ.get("XDG_CONFIG_HOME") or os.path.expanduser("~/.config"), "voiceloop")


def state_dir():
    """active_session and hook.log: %LOCALAPPDATA%\\VoiceLoop\\hook on Windows, else $XDG_STATE_HOME/voiceloop."""
    if os.environ.get("VOICELOOP_STATE"):
        return os.environ["VOICELOOP_STATE"]
    if os.name == "nt":
        return os.path.join(os.environ.get("LOCALAPPDATA") or os.path.expanduser("~"), "VoiceLoop", "hook")
    return os.path.join(os.environ.get("XDG_STATE_HOME") or os.path.expanduser("~/.local/state"), "voiceloop")


CONFIG_FILE = os.path.join(config_dir(), "hook.json")
STATE = state_dir()


def load_config():
    """(url, token). Environment variables win over hook.json. Either may be empty."""
    conf = {}
    try:
        with open(CONFIG_FILE, encoding="utf-8-sig") as f:
            conf = json.load(f)
    except (OSError, ValueError):
        pass
    url = os.environ.get("VOICELOOP_URL") or conf.get("url", "")
    token = os.environ.get("VOICELOOP_TOKEN") or conf.get("token", "")
    if not token and conf.get("token_file"):
        try:
            with open(os.path.expanduser(conf["token_file"])) as f:
                token = f.read().strip()
        except OSError as e:
            log(f"token_file unreadable: {e}")
    return url.rstrip("/"), token.strip()


def log(msg):
    try:
        os.makedirs(STATE, exist_ok=True)
        with open(os.path.join(STATE, "hook.log"), "a") as f:
            f.write(time.strftime("%Y-%m-%d %H:%M:%S ") + msg + "\n")
    except OSError:
        pass


def last_reply(data):
    text = data.get("last_assistant_message")
    if isinstance(text, str) and text.strip():
        return text
    path = data.get("transcript_path")
    if not path or not os.path.exists(path):
        return ""
    # Walk back from the end: collect assistant text until the last real user prompt.
    with open(path, "rb") as f:
        lines = f.read().decode("utf-8", "replace").splitlines()
    parts = []
    for line in reversed(lines):
        try:
            e = json.loads(line)
        except ValueError:
            continue
        m = e.get("message") or {}
        if e.get("type") == "assistant":
            content = m.get("content")
            if isinstance(content, list):
                t = "\n".join(c.get("text", "") for c in content if c.get("type") == "text")
                if t.strip():
                    parts.append(t)
        elif e.get("type") == "user":
            content = m.get("content")
            is_tool_result = isinstance(content, list) and any(
                c.get("type") == "tool_result" for c in content if isinstance(c, dict))
            if is_tool_result:
                continue
            text = content if isinstance(content, str) else " ".join(
                c.get("text", "") for c in (content or []) if isinstance(c, dict))
            if not parts and is_voiceloop_command(text):
                continue            # /resend itself may already be in the transcript
            break
    # Only the final text block is the answer; earlier ones are narration between tool calls.
    return parts[0] if parts else ""


# A slash command on its own: /resend, /mute, /repeat-last. Not and/or, 1/2 or paths.
SLASH_CMD_RE = re.compile(r"(?<![\w/.~])/([a-z][\w-]*)(?![\w/\\.])", re.I)


def spoken_code(c):
    """How to say an inline code span instead of dropping it."""
    c = c.strip()
    if not c:
        return ""
    if re.fullmatch(r"/[a-z][\w-]*", c, re.I):
        return "slash " + c[1:]                        # /resend -> slash resend
    if re.match(r"https?://", c):
        return "a link"
    def shorten(tok):
        # A path: say its last part. ~/code/voiceloop -> voiceloop, C:\x\voiceloop.exe -> voiceloop.exe
        if "\\" in tok or re.search(r"(^[~.]?/)|(\w/\w)", tok):
            return re.split(r"[\\/]+", tok.rstrip("/\\"))[-1] or "a path"
        return tok
    said = " ".join(shorten(t) for t in c.split())
    return "a command" if len(said) > 50 else said


def speakable(md):
    t = re.sub(r"```.*?```", " ", md, flags=re.S)           # code blocks
    out = []
    for line in t.splitlines():
        s = line.strip()
        if s.startswith("|") or re.fullmatch(r"[-:| ]+", s or "x"):
            continue                                         # tables, rules
        s = re.sub(r"^#{1,6}\s*", "", s)                     # headings
        s = re.sub(r"^([-*+]|\d+[.)])\s+", "", s)            # bullets
        out.append(s)
    t = "\n".join(out)
    t = re.sub(r"!?\[([^\]]*)\]\([^)]*\)", r"\1", t)         # links, images
    t = re.sub(r"https?://\S+", "a link", t)

    t = re.sub(r"`([^`]*)`", lambda m: spoken_code(m.group(1)), t)  # inline code
    t = SLASH_CMD_RE.sub(lambda m: "slash " + m.group(1), t)          # /mute in plain text
    t = re.sub(r"(\*\*|__|\*|_)(?=\S)(.+?)(?<=\S)\1", r"\2", t)  # bold/italic
    t = t.replace("→", " to ").replace("&", " and ")
    # Each line (paragraph line or bullet) becomes one or more sentences.
    sentences = []
    for line in t.splitlines():
        line = re.sub(r"\s+", " ", line).strip()
        line = re.sub(r"\s+([.,;:!?])", r"\1", line)          # gaps left by dropped paths
        if not line:
            continue
        if not re.search(r"[.!?:]$", line):
            line += "."                                      # bullets without a full stop
        sentences += [s for s in re.split(r"(?<=[.!?:])\s+", line) if s]
    # Fill up to the limit sentence by sentence, so a long list is read as far as it fits.
    picked = []
    for s in sentences:
        if picked and sum(len(x) + 1 for x in picked) + len(s) > MAX_CHARS:
            break
        picked.append(s)
    # Never finish on a lead-in like "Two things worth knowing:".
    while len(picked) > 1 and picked[-1].endswith(":"):
        picked.pop()
    spoken = " ".join(picked)
    return spoken[:MAX_CHARS * 2].strip()


def question_text(tool_input):
    """Spoken form of an AskUserQuestion call: the first question and its options."""
    qs = (tool_input or {}).get("questions") or []
    if not qs:
        return "Question."
    q = qs[0]
    labels = [o.get("label", "") for o in q.get("options", []) if o.get("label")]
    text = "Question. " + q.get("question", "").strip()
    if labels:
        opts = ", ".join(labels[:-1]) + (" or " if len(labels) > 1 else "") + labels[-1]
        text += " Options: " + opts + "."
    if len(qs) > 1:
        text += f" And {len(qs) - 1} more question{'s' if len(qs) > 2 else ''}."
    return text


def is_approval(data):
    kind = data.get("notification_type", "")
    msg = data.get("message", "")
    return kind == "permission_prompt" or "permission" in msg.lower()


T3_DB = os.path.expanduser("~/.t3/userdata/statev2.sqlite")

# "/mute", "/unmute", and the dictated forms "slash mute", "voice loop unmute" etc.
MUTE_RE = re.compile(r"^\s*(?:/\s*|slash\s+|voice\s*loop\s+)(un)?mute(?:\s+(?:this\s+)?thread)?[\s.!]*$", re.I)


# "/resend" (or /repeat, /replay, "slash resend"): read the thread's last reply again.
RESEND_RE = re.compile(r"^\s*(?:/\s*|slash\s+|voice\s*loop\s+)(?:resend|repeat|replay)(?:\s+(?:that|it|the\s+last\s+(?:reply|message|output)))?[\s.!]*$", re.I)


def is_voiceloop_command(text):
    return bool(RESEND_RE.match(text or "") or MUTE_RE.match(text or ""))


def thread_for(data):
    """The T3 thread (id, title) behind a Claude session, read-only from T3's database.
    Falls back to the session id and the project folder for sessions outside T3."""
    sid = data.get("session_id", "")
    fallback = sid, os.path.basename(data.get("cwd", "") or "") or "Claude"
    if not os.path.exists(T3_DB):
        return fallback                     # plain Claude Code, no T3
    try:
        import sqlite3
        db = sqlite3.connect(f"file:{T3_DB}?mode=ro", uri=True, timeout=1)
        try:
            row = db.execute(
                "select t.thread_id, t.title from orchestration_v2_projection_provider_threads p "
                "join orchestration_v2_projection_threads t on t.thread_id = p.thread_id "
                "where p.payload_json like ? order by p.updated_at desc limit 1", (f"%{sid}%",)).fetchone()
        finally:
            db.close()
        if row and row[0]:
            return row[0], (row[1] or "").strip()
    except Exception as e:
        log(f"thread lookup failed for {sid[:8]}: {e}")
    return fallback


class NotSetUp(Exception):
    pass


def post(path, payload):
    url, token = load_config()
    if not url or not token:
        raise NotSetUp(f"no address or token; run: {os.path.basename(__file__)} setup")
    req = urllib.request.Request(
        url + path, data=json.dumps(payload).encode(),
        headers={"Authorization": "Bearer " + token, "Content-Type": "application/json"},
        method="POST")
    with urllib.request.urlopen(req, timeout=3) as r:
        return r.status, r.read()


def block(reason):
    """Stop a prompt reaching Claude and show the user why (UserPromptSubmit only)."""
    print(json.dumps({"decision": "block", "reason": reason}))


def unreachable(e):
    if isinstance(e, NotSetUp):
        return "VoiceLoop isn't set up on this machine yet (run voiceloop_hook.py setup)"
    return "VoiceLoop isn't reachable on its PC"


def handle_mute(data, unmute):
    thread, title = thread_for(data)
    name = f"\u201c{title}\u201d" if title else "this thread"
    try:
        post("/mute", {"Thread": thread, "Title": title, "Muted": not unmute})
    except Exception as e:
        log(f"mute failed: {e}")
        block(f"{unreachable(e)}, so {name} wasn't {'unmuted' if unmute else 'muted'}.")
        return
    log(f"{'unmuted' if unmute else 'muted'} {thread[:8]} {title!r}")
    if unmute:
        block(f"VoiceLoop: {name} is unmuted. Its replies will be read out again.")
    else:
        block(f"VoiceLoop: {name} is muted. Type /unmute here to hear it again.")


def handle_resend(data):
    text = speakable(last_reply(data))
    if not text:
        block("VoiceLoop: there's no earlier reply in this thread to read out.")
        return
    thread, title = thread_for(data)
    try:
        # Force: the user asked for it, so play even if muted, not current, or voice mode is off.
        post("/speak", {"Text": text, "Kind": "speech", "Thread": thread, "Title": title,
                        "Active": True, "Force": True})
    except Exception as e:
        log(f"resend failed: {e}")
        block(f"{unreachable(e)}, so nothing was replayed.")
        return
    log(f"resend {title[:40]!r} {len(text)} chars")
    block("VoiceLoop: reading the last reply again.")


# ---- setup, remove, test: run by hand, never by Claude Code ----

USAGE = """usage:
  voiceloop_hook.py setup --url http://PC-ADDRESS:8765 --token TOKEN [--settings PATH]
  voiceloop_hook.py test
  voiceloop_hook.py remove [--settings PATH]

setup writes the address and token to {conf}
and adds the four hooks to Claude Code's settings.json (a dated backup is made first).
Settings, About in VoiceLoop shows the address and token."""

EVENTS = {  # Claude Code event -> (hook mode, matcher, timeout in seconds)
    "UserPromptSubmit": ("prompt", None, 5),
    "Stop": ("stop", None, 10),
    "PreToolUse": ("ask", "AskUserQuestion", 5),
    "Notification": ("notify", None, 5),
}


def claude_settings_path(args):
    if "--settings" in args:
        return os.path.expanduser(args[args.index("--settings") + 1])
    return os.path.join(os.environ.get("CLAUDE_CONFIG_DIR") or os.path.expanduser("~/.claude"), "settings.json")


def arg(args, name):
    if name in args and args.index(name) + 1 < len(args):
        return args[args.index(name) + 1]
    return ""


def hook_command(mode):
    q = lambda p: '"' + p + '"' if " " in p else p
    return f"{q(sys.executable)} {q(os.path.abspath(__file__))} {mode}"


def is_ours(entry):
    return any("voiceloop_hook.py" in (h.get("command") or "") for h in entry.get("hooks", []))


def edit_settings(path, add):
    """Remove every VoiceLoop hook from settings.json, then add ours back if add is set.
    Returns the backup path, or "" when the file didn't exist."""
    settings, backup = {}, ""
    if os.path.exists(path):
        with open(path, encoding="utf-8-sig") as f:
            text = f.read()
        settings = json.loads(text) if text.strip() else {}
        stem = path + time.strftime(".bak-voiceloop-%Y%m%d-%H%M%S")
        backup, n = stem, 1
        while os.path.exists(backup):        # never overwrite an earlier backup
            n += 1
            backup = f"{stem}-{n}"
        with open(backup, "w", encoding="utf-8") as f:
            f.write(text)
    hooks = settings.setdefault("hooks", {})
    for event in list(hooks):
        kept = [e for e in hooks[event] if not is_ours(e)]
        if kept:
            hooks[event] = kept
        else:
            del hooks[event]
    if add:
        for event, (mode, matcher, timeout) in EVENTS.items():
            entry = {"hooks": [{"type": "command", "command": hook_command(mode), "timeout": timeout}]}
            if matcher:
                entry = {"matcher": matcher, **entry}
            hooks.setdefault(event, []).append(entry)
    if not hooks:
        del settings["hooks"]
    os.makedirs(os.path.dirname(path) or ".", exist_ok=True)
    tmp = path + ".voiceloop-tmp"
    with open(tmp, "w", encoding="utf-8") as f:
        json.dump(settings, f, indent=2)
        f.write("\n")
    os.replace(tmp, path)
    return backup


def send_test():
    try:
        status, _ = post("/speak", {"Text": "VoiceLoop is paired with this machine.", "Kind": "speech",
                                    "Thread": "voiceloop-test", "Title": "VoiceLoop test",
                                    "Active": True, "Force": True})
        print(f"Test sent to {load_config()[0]} (HTTP {status}). You should hear it on the VoiceLoop PC.")
        return 0
    except NotSetUp:
        print("Not set up yet. Run: voiceloop_hook.py setup --url ... --token ...")
    except urllib.error.HTTPError as e:
        print(f"VoiceLoop answered HTTP {e.code}." + (" The token doesn't match: copy it again from Settings, About." if e.code == 401 else ""))
    except Exception as e:
        print(f"Couldn't reach VoiceLoop at {load_config()[0]}: {e}\n"
              "Check that it's running, the address is right, and the PC's firewall lets this machine in.")
    return 1


def admin(mode, args):
    if mode == "setup":
        url, token = arg(args, "--url").rstrip("/"), arg(args, "--token").strip()
        if not re.match(r"^https?://[^/\s]+$", url) or not token:
            print(USAGE.format(conf=CONFIG_FILE))
            return 2
        os.makedirs(config_dir(), exist_ok=True)
        fd = os.open(CONFIG_FILE, os.O_WRONLY | os.O_CREAT | os.O_TRUNC, 0o600)
        with os.fdopen(fd, "w", encoding="utf-8") as f:
            json.dump({"url": url, "token": token}, f, indent=2)
            f.write("\n")
        print(f"Saved the address and token to {CONFIG_FILE}")
        path = claude_settings_path(args)
        backup = edit_settings(path, add=True)
        print(f"Added the VoiceLoop hooks to {path}" + (f" (backup: {backup})" if backup else ""))
        print("New Claude Code sessions pick them up; restart any that are open.")
        return send_test()
    if mode == "remove":
        path = claude_settings_path(args)
        if not os.path.exists(path):
            print(f"{path} doesn't exist, so there's nothing to remove.")
            return 0
        backup = edit_settings(path, add=False)
        print(f"Removed the VoiceLoop hooks from {path} (backup: {backup})")
        print(f"The address and token are still in {CONFIG_FILE}; delete it if you're done with VoiceLoop.")
        return 0
    if mode == "test":
        return send_test()
    print(USAGE.format(conf=CONFIG_FILE))
    return 0 if mode else 2


def main():
    mode = sys.argv[1] if len(sys.argv) > 1 else ""
    if mode in ("setup", "remove", "test", "-h", "--help", "help", ""):
        sys.exit(admin(mode, sys.argv[2:]))
    try:
        data = json.load(sys.stdin)
    except ValueError:
        return
    sid = data.get("session_id", "")
    os.makedirs(STATE, exist_ok=True)
    active = os.path.join(STATE, "active_session")

    if mode == "prompt":
        prompt = data.get("prompt", "") or ""
        if RESEND_RE.match(prompt):
            handle_resend(data)
            return                  # nor does a resend
        m = MUTE_RE.match(prompt)
        if m:
            handle_mute(data, unmute=bool(m.group(1)))
            return                  # a mute command doesn't change the active thread
        with open(active, "w") as f:
            f.write(sid)
        return

    if mode not in ("stop", "ask", "notify"):
        return
    if mode == "stop" and data.get("stop_hook_active"):
        return
    try:
        current = open(active).read().strip()
    except OSError:
        current = ""
    if mode == "ask":
        kind, text = "question", question_text(data.get("tool_input"))
    elif mode == "notify":
        if not is_approval(data):
            log(f"skip {sid[:8]}: notification {data.get('notification_type', '?')}")
            return
        kind = "approval"
        text = "Approval needed. " + speakable(data.get("message", ""))
    else:
        kind, text = "speech", speakable(last_reply(data))
    if not text:
        log(f"skip {sid[:8]}: nothing speakable")
        return
    forked = hasattr(os, "fork")
    if forked:
        if os.fork() != 0:
            return                  # parent returns at once; Claude isn't kept waiting
        os.setsid()
    try:
        # The listener decides what to play: it knows the thread mode and the mute list.
        thread, title = thread_for(data)
        status, _ = post("/speak", {"Text": text, "Kind": kind, "Thread": thread,
                                    "Title": title, "Active": sid == current})
        log(f"sent {kind} {title[:40]!r} active={sid == current} {len(text)} chars -> {status}")
    except Exception as e:          # VoiceLoop PC asleep, listener closed: stay quiet
        log(f"send failed {sid[:8]}: {e}")
    if forked:
        os._exit(0)


if __name__ == "__main__":
    try:
        main()
    except Exception as e:
        log(f"error: {e}")
    sys.exit(0)

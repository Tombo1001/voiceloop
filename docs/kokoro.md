# Running Kokoro

**voiceloop** doesn't make speech itself. It sends text to [Kokoro-FastAPI](https://github.com/remsky/Kokoro-FastAPI), which wraps the open-source Kokoro model (82M parameters) in a Docker image with an OpenAI-style `/v1/audio/speech` endpoint. It runs on a CPU, and no API key or account is involved.

Kokoro-FastAPI is the default. From 0.8.2 any server with an OpenAI-style `/v1/audio/speech` endpoint works too: see [Other speech servers](#other-speech-servers).

## Where to run it

Anywhere the **voiceloop** PC can reach over HTTP:

- **On the **voiceloop** PC itself**, with Docker Desktop. The default address in Settings, `http://127.0.0.1:8880`, then works as it is.
- **On another box** with spare CPU, such as a home server or NAS. Put its address in Settings, Voice.

It needs about 2 GB of memory and a few CPU cores.

## Start it

`kokoro/docker-compose.yml` in this repo is the CPU image with a 3 GB memory cap and 3 cores:

```bash
cd kokoro
docker compose up -d
curl http://localhost:8880/health
```

Without compose:

```bash
docker run -d --name kokoro-tts --restart unless-stopped \
  --memory 3g --memory-swap 3g --cpus 3 \
  -p 8880:8880 ghcr.io/remsky/kokoro-fastapi-cpu:v0.9.2
```

The first start downloads about 3.3 GB, and the model then takes a little under a minute to load. Kokoro-FastAPI also has a GPU image; its README has the details.

In **voiceloop**, open Settings, Voice, enter the address and press **Test**. The voice list fills in when the test passes. Voice names start with a language and gender code: `bf_` is British female, `bm_` British male, `af_` American female. The default is `bf_emma`.

## Numbers

These were measured on 3 cores of a Ryzen 5 5600 that was also running other services:

- About 1 s to synthesise 40 characters, and about 3.6 s for 180 characters.
- A 935-character reply took 19 s as one request. **voiceloop** splits replies into sentence chunks and plays each one while the next is made, so speech starts about 1.3 s after the reply arrives.
- Memory sat at 1.6 to 2.2 GB. With a 2.5 GB cap, Docker killed it for running out of memory several times in one evening, which is why the compose file uses 3 GB. When Kokoro restarts mid-reply, **voiceloop** retries for about 20 seconds.

## Keep it private

Kokoro-FastAPI has no authentication. Don't expose port 8880 to the internet. Keep it on your LAN or a tailnet.

## Other speech servers

From 0.8.2, **voiceloop** works with any server that has an OpenAI-style `POST /v1/audio/speech` endpoint and can return WAV. Settings, Voice has three boxes for it:

- **Address**, for example `http://my-server:8000`.
- **Model**, the name the server expects. It's `kokoro` for Kokoro-FastAPI.
- **API key**, only if the server asks for one. It's sent as a bearer token, stays on the PC and is left out of settings exports.

**Test connection** tries `/health`, then `/v1/models`, then `/v1/audio/voices`. If the server has a voice list at `/v1/audio/voices`, the voice picker fills from it. If not, the picker turns into a text box and you type the voice name from the server's docs. **voiceloop** converts whatever WAV comes back (any sample rate, stereo, 8 to 32-bit or float) before playing it.

Tested on 2026-11-10:

- **Kokoro-FastAPI** v0.9.2, model `kokoro`, voice `bf_emma`.
- **[Speaches](https://github.com/speaches-ai/speaches)** (`ghcr.io/speaches-ai/speaches:latest-cpu`) with a Piper voice. Download the model into Speaches first with `curl -X POST http://my-server:8000/v1/models/speaches-ai/piper-en_GB-alba-medium`, then set the model to `speaches-ai/piper-en_GB-alba-medium` and the voice to `alba`. Speaches can also run Kokoro (`speaches-ai/Kokoro-82M-v1.0-ONNX`, voice `af_heart`). Its own docs list the other models.

These have the same endpoint, going by their docs and code, but haven't been tried with **voiceloop**:

- **[LocalAI](https://github.com/mudler/LocalAI)**: the model is whichever TTS model you installed in it.
- **[Chatterbox TTS Server](https://github.com/devnen/Chatterbox-TTS-Server)**: voices are file names such as `Emily.wav`.
- **[Orpheus-FastAPI](https://github.com/Lex-au/Orpheus-FastAPI)**: model `orpheus`, voices such as `tara`. It needs a separate llama.cpp server.

If one of these, or another server, works or fails for you, [open an issue](https://github.com/Tombo1001/voiceloop/issues) with what you set.

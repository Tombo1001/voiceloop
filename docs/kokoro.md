# Running Kokoro

VoiceLoop doesn't make speech itself. It sends text to [Kokoro-FastAPI](https://github.com/remsky/Kokoro-FastAPI), which wraps the open-source Kokoro model (82M parameters) in a Docker image with an OpenAI-style `/v1/audio/speech` endpoint. It runs on a CPU, and no API key or account is involved.

Kokoro-FastAPI is the only server VoiceLoop has been tested with. VoiceLoop asks for model `kokoro` and checks the server at `/health`, so other OpenAI-style speech servers may not work yet.

## Where to run it

Anywhere the VoiceLoop PC can reach over HTTP:

- **On the VoiceLoop PC itself**, with Docker Desktop. The default address in Settings, `http://127.0.0.1:8880`, then works as it is.
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

In VoiceLoop, open Settings, Voice, enter the address and press **Test**. The voice list fills in when the test passes. Voice names start with a language and gender code: `bf_` is British female, `bm_` British male, `af_` American female. The default is `bf_emma`.

## Numbers

These were measured on 3 cores of a Ryzen 5 5600 that was also running other services:

- About 1 s to synthesise 40 characters, and about 3.6 s for 180 characters.
- A 935-character reply took 19 s as one request. VoiceLoop splits replies into sentence chunks and plays each one while the next is made, so speech starts about 1.3 s after the reply arrives.
- Memory sat at 1.6 to 2.2 GB. With a 2.5 GB cap, Docker killed it for running out of memory several times in one evening, which is why the compose file uses 3 GB. When Kokoro restarts mid-reply, VoiceLoop retries for about 20 seconds.

## Keep it private

Kokoro-FastAPI has no authentication. Don't expose port 8880 to the internet. Keep it on your LAN or a tailnet.

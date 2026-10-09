# go-voice-bot-ce

Real-time AI voice agent for Comcent Community Edition. Written in Go.
Integrates with Deepgram (speech-to-text + text-to-speech) and OpenAI
(LLM reasoning), served over SIP to a comcent-ce instance.

## Quick start

Requires Go 1.23+, a running comcent-ce server, and API keys for Deepgram
and OpenAI.

```bash
cp .env.example .env              # fill in keys
go build ./...
./voice-bot
```

## Configuration

| env var | purpose |
|---|---|
| `COMCENT_API_URL` | base URL of your comcent-ce server |
| `COMCENT_API_KEY` | org API key from comcent-ce settings |
| `DEEPGRAM_API_KEY` | Deepgram account key |
| `OPENAI_API_KEY` | OpenAI account key |
| `SIP_REGISTER_URL` | SIP server to register against |

## Releasing

Every push to `main` publishes the image as `:main` and an immutable
`:sha-<7>`. It never moves `:latest` or a version tag.

A release is made by hand, after the `:sha-<7>` build has been tested with
comcent-ce:

1. Actions → **Release** → *Run workflow*, with `version` `vYYYY.MM.DD` for
   today (add `.1`, `.2` for another release the same day) and `sha` the
   commit you tested (blank means current `main`). It checks the
   `:sha-<7>` image exists, tags it `:<version>` and `:latest`, tags the
   commit and creates the GitHub Release. Never create a Release any other
   way: only this workflow gives the image its version tag.
2. In comcent-ce, open a PR pinning `go-voice-bot-ce:<version>` (see its
   `RELEASING.md`). Installs get it with comcent-ce's next release.

## License

AGPL-3.0. Commercial licenses available — contact the maintainer.

Contributors: a CLA signature is required before your PR is merged.

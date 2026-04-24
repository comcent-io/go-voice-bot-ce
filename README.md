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

## License

AGPL-3.0. Commercial licenses available — contact the maintainer.

Contributors: a CLA signature is required before your PR is merged.

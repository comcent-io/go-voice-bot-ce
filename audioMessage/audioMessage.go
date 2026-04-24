package audioMessage

// RealtimeBillingDetails represents billing details for realtime API usage
type RealtimeBillingDetails struct {
	InputTextTokens        int `json:"inputTextTokens"`
	InputAudioTokens       int `json:"inputAudioTokens"`
	CachedInputTextTokens  int `json:"cachedInputTextTokens"`
	CachedInputAudioTokens int `json:"cachedInputAudioTokens"`
	OutputTextTokens       int `json:"outputTextTokens"`
	OutputAudioTokens      int `json:"outputAudioTokens"`
}

// AudioMessage represents a message with type, text, audio data, and command
type AudioMessage struct {
	Type                   string                 `json:"type"`
	Text                   string                 `json:"text"`
	Audio                  []byte                 `json:"audio"`
	Command                string                 `json:"command"`
	SttDuration            float64                `json:"sttDuration"`
	GptInputTokens         int                    `json:"gptInputTokens"`
	GptOutputTokens        int                    `json:"gptOutputTokens"`
	TtsCharacters          int                    `json:"ttsCharacters"`
	RealtimeBillingDetails RealtimeBillingDetails `json:"realtimeBillingDetails"`
}

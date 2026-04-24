package audio

import (
	_ "embed"
	"math/rand"
)

//go:embed phone_ringing_G711.org_.wav
var PhoneRingingAudio []byte

//go:embed audio1_G711.org_.wav
var InProgressAudio1 []byte

//go:embed audio2_G711.org_.wav
var InProgressAudio2 []byte

//go:embed audio3_G711.org_.wav
var InProgressAudio3 []byte

//go:embed audio4_G711.org_.wav
var InProgressAudio4 []byte

//go:embed audio5_G711.org_.wav
var InProgressAudio5 []byte

//go:embed audio6_G711.org_.wav
var InProgressAudio6 []byte

//go:embed ourServiceIsUnavailable_G711.org_.wav
var ServiceUnavailableAudio []byte

//go:embed someThingWentWrong_G711.org_.wav
var SomethingWentWrongAudio []byte

//go:embed ack-1_G711.org_.wav
var Ack1Audio []byte

//go:embed ack-2_G711.org_.wav
var Ack2Audio []byte

//go:embed ack-3_G711.org_.wav
var Ack3Audio []byte

//go:embed ack-4_G711.org_.wav
var Ack4Audio []byte

//go:embed ack-5_G711.org_.wav
var Ack5Audio []byte

//go:embed in_progress_G711.org_.wav
var InProgressAudio []byte

func RandomInProgressAudio() []byte {
	inProgressAudios := [][]byte{InProgressAudio1, InProgressAudio2, InProgressAudio3, InProgressAudio4, InProgressAudio5, InProgressAudio6}
	return inProgressAudios[rand.Intn(len(inProgressAudios))][44:]
}

func RandomAckAudio() []byte {
	ackAudios := [][]byte{Ack1Audio, Ack2Audio, Ack3Audio, Ack4Audio, Ack5Audio}
	return ackAudios[rand.Intn(len(ackAudios))][44:]
}

func GetRawBytes(data *[]byte) []byte {
	return (*data)[44:]
}

func GetInProgressAudio() []byte {
	return InProgressAudio[44:]
}

type WavePlayer struct {
	data       *[]byte
	index      int
	streamSize int
}

func NewAudioPlayer(data *[]byte, streamSize int) *WavePlayer {
	return &WavePlayer{
		data:       data,
		index:      44,
		streamSize: streamSize,
	}
}

func (p *WavePlayer) GetNextBytes() []byte {
	var result []byte
	if p.index+p.streamSize <= len(*p.data) {
		result = (*p.data)[p.index : p.index+p.streamSize]
		p.index += p.streamSize
	} else {
		result = append(result, (*p.data)[p.index:]...)
		sampleSize := len(*p.data) - p.index
		p.index = 44
		if sampleSize > 0 {
			result = append(result, (*p.data)[p.index:p.index+sampleSize]...)
		}
	}
	return result
}

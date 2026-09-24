package ape

import "errors"

// File and container sizes from the Monkey's Audio 3.99 descriptor.
const (
	fileVersion    = 3990
	version3980    = 3980
	version3950    = 3950
	version3930    = 3930
	oldHeaderSize  = 32
	flag8Bit       = 1 << 0
	flagHasPeak    = 1 << 2
	flag24Bit      = 1 << 3
	flagHasSeek    = 1 << 4
	programVersion = 13*1000 + 26*10 // SDK 13.26, written in the descriptor.

	descriptorSize = 52
	headerSize     = 24

	flagCreateWAV = 1 << 5
	flagAIFF      = 1 << 6
	flagW64       = 1 << 7
	flagSND       = 1 << 8
	flagCAF       = 1 << 10
	flagFloat     = 1 << 12

	maxChannels = 32

	blocksFast   = 73728
	blocksExtra  = blocksFast * 4
	blocksInsane = blocksFast * 16

	bytesPerSample16 = 2
	wordSize         = 4
	crcHighBit       = 31
)

// Compression is a Monkey's Audio compression level.
type Compression uint16

// Compression levels. The number is the value stored in the APE header.
const (
	CompressionFast      Compression = 1000
	CompressionNormal    Compression = 2000
	CompressionHigh      Compression = 3000
	CompressionExtraHigh Compression = 4000
	CompressionInsane    Compression = 5000
)

// Stream is the PCM layout Encode accepts.
type Stream struct {
	SampleRate int
	Channels   int // 1 to 32
	Bits       int // 8, 16, 24, or 32
	// Float marks 32-bit samples as IEEE float. The file magic becomes MACF.
	Float bool
}

// Format tells a decoder what Header and Footer contain.
type Format uint16

// Container kinds stored with Header and Footer. WAV is the zero value.
const (
	FormatWAV  Format = 0
	FormatAIFF Format = flagAIFF
	FormatW64  Format = flagW64
	FormatSND  Format = flagSND
	FormatCAF  Format = flagCAF
)

// Options selects the encoder. A zero Options value is fast compression
// at the level's normal frame size.
type Options struct {
	Compression Compression
	// BlocksPerFrame overrides the frame size. Zero uses the level default.
	// Tests use a short frame; files other people play should leave this zero.
	BlocksPerFrame int
	// Header and Footer are stored ahead of and behind the audio.
	// An empty Header asks the decoder to build a WAV header instead.
	Header []byte
	Footer []byte
	Format Format
	// Tags are written as an APEv2 tag after the footer.
	Tags map[string]string
}

// ErrUnsupported is returned for a level, bit depth, or channel count that
// this build does not encode yet.
var ErrUnsupported = errors.New("ape: unsupported encode")

var (
	errShortDescriptor = errors.New("ape: short descriptor")
	errEmptyAudio      = errors.New("ape: empty audio")
	errShortFrame      = errors.New("ape: short frame")
	errShortSeekCount  = errors.New("ape: short seek count")
	errShortSeekTable  = errors.New("ape: short seek table")
	errLinkStart       = errors.New("ape: link starts past the audio")
	errLinkEnd         = errors.New("ape: link ends past the audio")
)

var errPCMLength = errors.New("ape: pcm length is not a whole number of frames")

type nnStage struct {
	order int
	shift int
}

func (c Compression) stages() []nnStage {
	switch c {
	case CompressionFast:
		return nil
	case CompressionNormal:
		return []nnStage{{order: nnOrderNormal, shift: nnShiftNormal}}
	case CompressionHigh:
		return []nnStage{{order: 64, shift: 11}}
	case CompressionExtraHigh:
		return []nnStage{{order: 256, shift: 13}, {order: 32, shift: 10}}
	case CompressionInsane:
		return []nnStage{{order: 1280, shift: 15}, {order: 256, shift: 13}, {order: nnOrderNormal, shift: nnShiftNormal}}
	default:
		return nil
	}
}

func (c Compression) blocksPerFrame() (int, error) {
	switch c {
	case CompressionFast, CompressionNormal, CompressionHigh:
		return blocksFast, nil
	case CompressionExtraHigh:
		return blocksExtra, nil
	case CompressionInsane:
		return blocksInsane, nil
	default:
		return 0, ErrUnsupported
	}
}

func (o *Options) compression() Compression {
	if o == nil || o.Compression == 0 {
		return CompressionFast
	}

	return o.Compression
}

func (o *Options) frameBlocks(level Compression) (int, error) {
	def, err := level.blocksPerFrame()
	if err != nil {
		return 0, err
	}

	if o != nil && o.BlocksPerFrame > 0 {
		return o.BlocksPerFrame, nil
	}

	return def, nil
}

func (s Stream) sampleBytes() int { return s.Bits / 8 }

func (s Stream) blockAlign() int { return s.Channels * s.sampleBytes() }

func validate(pcm []byte, stream Stream) error {
	switch stream.Bits {
	case 8, 16, 24, 32:
	default:
		return ErrUnsupported
	}

	if stream.Float && stream.Bits != 32 {
		return ErrUnsupported
	}

	if stream.Channels < 1 || stream.Channels > maxChannels || stream.SampleRate <= 0 {
		return ErrUnsupported
	}

	block := stream.blockAlign()
	if len(pcm) == 0 || len(pcm)%block != 0 {
		return errPCMLength
	}

	return nil
}

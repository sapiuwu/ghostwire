package protocol

const (
	ProtocolMagic     uint32 = 0x47575244
	ProtocolVersion   uint8  = 1
	HeaderSize        int    = 12
	MaxPayloadSize    int    = 32 * 1024 * 1024
	FrameMetaSize     int    = 32
	InputEventSize    int    = 20
	MaxFrameRawSize   int    = 64 * 1024 * 1024
)

type MessageKind uint8

const (
	MsgHello      MessageKind = 1
	MsgWelcome    MessageKind = 2
	MsgFrame      MessageKind = 3
	MsgInput      MessageKind = 4
	MsgPing       MessageKind = 5
	MsgPong       MessageKind = 6
	MsgError      MessageKind = 7
	MsgBye        MessageKind = 8
	MsgScreenInfo MessageKind = 9
)

type HeaderFlag uint16

const (
	FlagDeflate HeaderFlag = 1 << 0
)

type PixelFormat uint8

const (
	PixelBGRA8 PixelFormat = 0
)

type FrameEncoding uint8

const (
	FrameRaw     FrameEncoding = 0
	FrameDeflate FrameEncoding = 1
)

const (
	ServerVersion      = "0.1.0"
	ClientNameDefault  = "ghostwire-cli"
)

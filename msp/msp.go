package msp

import (
	"bytes"
	"encoding/binary"
	"encoding/hex"
	"fmt"
	"github.com/sirupsen/logrus"
	//"github.com/tarm/serial"
	"io"
	"reflect"
)

const (
	ApiVersion               = 1 //out message
	FcVariant                = 2 //out message
	FcVersion                = 3 //out message
	BoardInfo                = 4 //out message
	BuildInfo                = 5 //out message
	InavPid                  = 6
	SetInavPid               = 7
	Name                     = 10 //out message     returns user set board name - betaflight
	SetName                  = 11 //in message      sets board name - betaflight
	NavPoshold               = 12
	SetNavPoshold            = 13
	PositionEstimationConfig = 16
	WpMissionLoad            = 18 //in message 		load mission from NVRAM
	WpMissionSave            = 19 //int message 	save mission to NVRAM
	WpGetinfo                = 20
	RthAndLandConfig         = 21
	SetRthAndLandConfig      = 22
	FwConfig                 = 23
	SetFwConfig              = 24
	Feature                  = 36
	SetFeature               = 37
	CFSerialConfig           = 54
	SetCFSerialConfig        = 55
	CurrentMeterConfig       = 40
	CfSerialConfig           = 54
	SetCfSerialConfig        = 55
	VoltageMeterConfig       = 56
	SonarAltitude            = 58  //out message	get surface altitude [cm]
	Reboot                   = 68  //in message		reboot settings
	Status                   = 101 //out message    cycletime & errors_count & sensor present & box activation & current setting number
	RawGps                   = 106 //out message    fix, numsat, lat, lon, alt, speed, ground course
	CompGps                  = 107 //out message    distance home, direction home
	Attitude                 = 108 //out message    2 angles 1 heading
	Altitude                 = 109 //out message    altitude, variometer
	Analog                   = 110 //out message    vbat, powermetersum, rssi if available on RX
	Wp                       = 118 //out message    get a WP, WP# is in the payload, returns (WP#, lat, lon, alt, flags) WP#0-home, WP#16-poshold
	NavStatus                = 121 //out message    Returns navigation status
	NavConfig                = 122 //out message    Returns navigation parameters
	SetWp                    = 209 //in message     sets a given WP (WP#,lat, lon, alt, flags)
	EepromWrite              = 250 //in message     no param
	Debugmsg                 = 253 //out message    debug string buffer
	Debug                    = 254 //out message    debug1,debug2,debug3,debug4
	StatusEx                 = 150 //out message    cycletime, errors_count, CPU load, sensor present etc
	SensorStatus             = 151 //out message    hardware sensor status
	Uid                      = 160 //out message    unique device ID
	GpsSvInfo                = 164 //out message    get Signal Strength (only U-Blox)
	GpsStatistics            = 166 //out message    get GPS debugging data
	DebugMsg                 = 253
	V2Frame                  = 255 //MSPv2 payload indicator
)

type InvalidPacketError struct {}

func (e *InvalidPacketError) Error() string {
	return "Invalid Packet"
}

func mspV2Encode(cmd uint16, payload []byte) []byte {
	leBuffer := make([]byte, 2)
	payloadLength := uint16(len(payload))

	var buf bytes.Buffer
	buf.WriteByte('$')
	buf.WriteByte('X')
	buf.WriteByte('<')
	buf.WriteByte(0)
	binary.LittleEndian.PutUint16(leBuffer, cmd)
	buf.Write(leBuffer)
	binary.LittleEndian.PutUint16(leBuffer, payloadLength)
	buf.Write(leBuffer)
	if payloadLength > 0 {
		buf.Write(payload[:payloadLength])
	}

	crc := byte(0)
	for _, v := range buf.Bytes()[3:] {
		crc = crc8DvbS2(crc, v)
	}
	buf.WriteByte(crc)
	return buf.Bytes()
}

func crc8DvbS2(crc, a byte) byte {
	crc ^= a
	for ii := 0; ii < 8; ii++ {
		if (crc & 0x80) != 0 {
			crc = (crc << 1) ^ 0xD5
		} else {
			crc = crc << 1
		}
	}
	return crc
}

type MSP struct {
	Port     io.ReadWriter
	logger   *logrus.Logger
}

type Frame struct {
	Code       uint16
	Payload    []byte
	payloadPos int
}



// Reads out from the frame Payload and advances the payload
// position pointer by the size of the variable pointed by out.
func (f *Frame) Read(out interface{}) error {
	switch x := out.(type) {
	case *uint8:
		if f.BytesRemaining() < 1 {
			return io.EOF
		}
		*x = f.Payload[f.payloadPos]
		f.payloadPos++
	case *uint16:
		if f.BytesRemaining() < 2 {
			return io.EOF
		}
		*x = binary.LittleEndian.Uint16(f.Payload[f.payloadPos:])
		f.payloadPos += 2
	case *uint32:
		if f.BytesRemaining() < 4 {
			return io.EOF
		}
		*x = binary.LittleEndian.Uint32(f.Payload[f.payloadPos:])
		f.payloadPos += 4
	default:
		v := reflect.ValueOf(out)
		if v.Kind() == reflect.Ptr && v.Elem().Kind() == reflect.Struct {
			elem := v.Elem()
			for i := 0; i < elem.NumField(); i++ {
				if err := f.Read(elem.Field(i).Addr().Interface()); err != nil {
					return err
				}
			}
			return nil
		}
		if v.Kind() == reflect.Slice {
			for i := 0; i < v.Len(); i++ {
				if err := f.Read(v.Index(i).Addr().Interface()); err != nil {
					return err
				}
			}
			return nil
		}
		panic(fmt.Errorf("can't decode MSP payload into type %v", out))
	}
	return nil
}

func (f *Frame) BytesRemaining() int {
	return len(f.Payload) - f.payloadPos
}

func New(port io.ReadWriter, logger *logrus.Logger) (*MSP, error) {
	return &MSP{
		Port:     port,
		logger:   logger,
	}, nil
}

func EncodeArgs(w *bytes.Buffer, args ...interface{}) error {
	for _, arg := range args {
		switch x := arg.(type) {
		case uint8:
			w.WriteByte(x)
		case uint16:
		case uint32:
			_ = binary.Write(w, binary.LittleEndian, x)

		default:
			v := reflect.ValueOf(arg)
			if v.Kind() == reflect.Slice {
				for i := 0; i < v.Len(); i++ {
					if err := EncodeArgs(w, v.Index(i).Interface()); err != nil {
						return err
					}
				}
				return nil
			}
			if v.Kind() == reflect.Struct {
				for i := 0; i < v.NumField(); i++ {
					if err := EncodeArgs(w, v.Field(i).Interface()); err != nil {
						return err
					}
				}
				return nil
			}
			panic(fmt.Errorf("can't encode MSP value of type %T", arg))
		}
	}
	return nil
}

func (m *MSP) WriteCmd(cmd uint16, args ...interface{}) (int, error) {
	var buf bytes.Buffer
	if err := EncodeArgs(&buf, args...); err != nil {
		return -1, err
	}
	data := buf.Bytes()
	frame := mspV2Encode(cmd, data)

	dst := make([]byte, hex.EncodedLen(len(frame)))
	hex.Encode(dst, frame)
	m.logger.Debugf("< %s\n", dst)

	return m.Port.Write(frame)
}

func (m *MSP) ReadFrame() (*Frame, error) {
	buf := make([]byte, 8)

	_, err := m.Port.Read(buf[0:3])
	if err != nil {
		return nil, err
	}

	if buf[0] != '$' {
		return nil, fmt.Errorf("invalid MSP header char 0x%02x buffer: %v %w", buf[0], buf, &InvalidPacketError{})
	}

	if buf[2] != '<' && buf[2] != '>' {
		return nil, fmt.Errorf("invalid MSP direction char 0x%02x buffer: %v %w", buf[2], buf, &InvalidPacketError{})
	}

	switch buf[1] {
	case 'M':
		return nil, fmt.Errorf("got MSP V1 message ignoring%w", &InvalidPacketError{})
	case 'X':
		_, err := m.Port.Read(buf[3:])
		if err != nil {
			return nil, err
		}
		code := uint16(buf[4]) | uint16(buf[5])<<8
		payloadLength := uint16(buf[6]) | uint16(buf[7])<<8

		var payload []byte
		if payloadLength > 0 {
			payload = make([]byte, payloadLength)
			_, err := io.ReadFull(m.Port, payload)
			if err != nil {
				return nil, err
			}
		}

		buf = append(buf, payload...)

		checksum := make([]byte, 1)
		if _, err := m.Port.Read(checksum); err != nil {
			return nil, err
		}

		dst := make([]byte, hex.EncodedLen(len(buf)))
		hex.Encode(dst, buf)
		m.logger.Debugf("> %s\n", dst)

		crc := byte(0)
		for _, v := range buf[3:] {
			crc = crc8DvbS2(crc, v)
		}
		if crc != checksum[0] {
			return nil, fmt.Errorf("invalid CRC 0x%02x, expecting 0x%02x in cmd %v with payload %v%w",
				checksum[0], crc, code, payload, &InvalidPacketError{})
		}
		return &Frame{
			Code:    code,
			Payload: payload,
		}, nil
	default:
		return nil, fmt.Errorf("unknown MSP char %c%w", buf[0], &InvalidPacketError{})
	}
}
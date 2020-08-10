package msp

type CFSerialConfigData struct {
	Identifier              uint8
	FunctionMask            uint16
	MSPBaudRateIndex        uint8
	GPSBaudRateIndex        uint8
	TelemetryBaudRateIndex  uint8
	PeripheralBaudRateIndex uint8 // Actually blackboxBaudRateIndex in BF
}

type SetGetWpData struct {
	WpNo      uint8
	Action    uint8
	Latitude  uint32
	Longitude uint32
	Altitude  uint32
	P1        uint16
	P2        uint16
	P3        uint16
	Flag      uint8
}

type RawGpsData struct {
	FixType      uint8
	NumSat       uint8
	Latitude     uint32
	Longitude    uint32
	Altitude     uint16
	GroundSpeed  uint16
	GroundCourse uint16
	Hdop         uint16
}

type NavStatusData struct {
	Mode uint8
	State uint8
	ActiveWpAction uint8
	ActiveWpNumber uint8
	Error uint8
	HeadingHoldToTarget uint16
}

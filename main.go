package main

import (
	"github.com/gtu-nova/nova-cli/msp"
	geo "github.com/kellydunn/golang-geo"
	"github.com/tarm/serial"
	"strings"
	"time"

	"flag"
	"github.com/gtu-nova/nova-cli/fc"
	"github.com/sirupsen/logrus"
	"os"
)

var (
	portName = flag.String("p", "", "Serial port")
)

var logger = logrus.New()

func initLogger() {
	f, err := os.OpenFile("log", os.O_WRONLY | os.O_APPEND, 0755)
	if err != nil {
		logger.Fatalf("Failed to open log file")
	}
	logger.SetOutput(f)
	logger.SetLevel(logrus.DebugLevel)
}

type myFC struct {
	variant      string
	versionMajor byte
	versionMinor byte
	versionPatch byte
	boardID      string
	targetName   string
	Features     uint32
}

type NavPoint struct {
	distance float64
	degree float64
}

type PosHoldNavSystem struct {
	points []NavPoint
	pointCount uint8
}

func (f *myFC) printInfo() {
	if f.variant != "" && f.versionMajor != 0 && f.boardID != "" {
		targetName := ""
		if f.targetName != "" {
			targetName = ", target " + f.targetName
		}
		logger.Infof("%s %d.%d.%d (board %s%s)\n", f.variant, f.versionMajor, f.versionMinor, f.versionPatch, f.boardID, targetName)
	}
}

func (f *myFC) versionGte(major, minor, patch byte) bool {
	return f.versionMajor > major || (f.versionMajor == major && f.versionMinor > minor) ||
		(f.versionMajor == major && f.versionMinor == minor && f.versionPatch >= patch)
}

// HasDetectedTargetName returns true if the target name installed on
// the board has been retrieved via MSP.
func (f *myFC) HasDetectedTargetName() bool {
	return f.targetName != ""
}

var _fc myFC

func main() {
	flag.Parse()
	initLogger()

	if *portName == "" {
		logger.Fatal("Missing port\n")
	}

	opts := &serial.Config{
		Name: *portName,
		Baud: 115200,
	}
	port, err := serial.OpenPort(opts)
	if err != nil {
		logger.Fatalf("Can't open port (%v)\n", err)
	}


	//port, err := net.Dial("tcp", "gtu-nova.local:8000")
	//if err != nil {
	//	fmt.Println(err)
	//	return
	//}

	fcv, err := fc.NewFC(port, logger)
	if err != nil {
		logger.Fatal(err)
	}


	//ticker := time.NewTicker(200 * time.Millisecond)

	fr, _ := fcv.WriteCmd(msp.ApiVersion)
	if fr != nil {
		logger.Infof("MSP API version %d.%d (protocol %d)\n", fr.Payload[1], fr.Payload[2], fr.Payload[0])
	}

	fr, _ = fcv.WriteCmd(msp.FcVariant)
	if fr != nil {
		_fc.variant = string(fr.Payload)
		_fc.printInfo()
	}

	fr, _ = fcv.WriteCmd(msp.FcVersion)
	if fr != nil {
		_fc.versionMajor = fr.Payload[0]
		_fc.versionMinor = fr.Payload[1]
		_fc.versionPatch = fr.Payload[2]
		_fc.printInfo()
	}

	fr, _ = fcv.WriteCmd(msp.BoardInfo)
	if fr != nil {
		// BoardID is always 4 characters
		_fc.boardID = string(fr.Payload[:4])
		// Then 4 bytes follow, HW revision (uint16), builtin OSD type (uint8) and wether
		// the board uses VCP (uint8), We ignore those bytes here. Finally, in recent BF
		// and iNAV versions, the length of the targetName (uint8) followed by the target
		// name itself is sent. Try to retrieve it.
		if len(fr.Payload) >= 9 {
			targetNameLength := int(fr.Payload[8])
			if len(fr.Payload) > 8+targetNameLength {
				_fc.targetName = string(fr.Payload[9 : 9+targetNameLength])
			}
		}
		_fc.printInfo()
	}


	fr, _ = fcv.WriteCmd(msp.BuildInfo)
	if fr != nil {
		buildDate := string(fr.Payload[:11])
		buildTime := string(fr.Payload[11:19])
		// XXX: Revision is 8 characters in iNav but 7 in BF/CF
		rev := string(fr.Payload[19:])
		logger.Infof("Build %s (built on %s @ %s)\n", rev, buildDate, buildTime)
	}

	for {
		fr, _ = fcv.WriteCmd(msp.NavStatus)
		if fr != nil {
			var r msp.NavStatusData
			_ = fr.Read(&r)
			if r.Mode != 0 {
				break
			}
		}
		time.Sleep(time.Second)
	}

	logger.Warn("Entered navigation mode starting mission\n")

	mission := PosHoldNavSystem{
		points:     []NavPoint{{
			distance: 0.02,
			degree:   20,
		},
		{
			distance: 0.02,
			degree:   340,
		}},
	}



	fr, _ = fcv.WriteCmd(msp.Wp, msp.SetGetWpData{
		WpNo: 0,
	})
	if fr != nil {
		var r msp.SetGetWpData
		_ = fr.Read(&r)
		logger.Infof("%+v\n", r)

		lat := float64(r.Latitude) / 10_000_000
		long := float64(r.Longitude) / 10_000_000

		point := geo.NewPoint(lat, long)
		var newPoint *geo.Point

		for i := 0; i < len(mission.points);{
			fr, _ = fcv.WriteCmd(msp.RawGps)
			if fr != nil {
				var gps msp.RawGpsData
				_ = fr.Read(&gps)
				currentPos := geo.NewPoint(float64(gps.Latitude) / 10_000_000, float64(gps.Longitude) / 10_000_000)
				dist := currentPos.GreatCircleDistance(newPoint)
				if dist / 1000 < 5 {
					logger.Infof("Reached the current goal setting next goal")
					i++
					newPoint = point.PointAtDistanceAndBearing(mission.points[i].distance, mission.points[i].degree)
					fr, _ = fcv.WriteCmd(msp.SetWp, msp.SetGetWpData{
						WpNo:     0,
						Action:    	1,
						Latitude:  uint32(newPoint.Lat() * 10_000_000),
						Longitude: uint32(newPoint.Lng() * 10_000_000),
						Altitude:  1000,
						Flag: 0xa5,
					})
				}
			}
		}

		fr, err = fcv.WriteCmd(msp.Wp, uint8(1))
		if fr != nil {
			var rx msp.SetGetWpData
			_ = fr.Read(&rx)
			logger.Infof("No: %d, Lat: %f, Lon: %f, Alt: %d, Speed: %d\n", rx.WpNo, float64(rx.Latitude)/10_000_000, float64(rx.Longitude)/10_000_000, rx.Altitude, rx.P1)
		}
	}

}

func handleFrame(fr msp.Frame, fc *fc.FC) error {
	switch fr.Code {

	case msp.DebugMsg:
		s := strings.Trim(string(fr.Payload), " \r\n\t\x00")
		logger.Debugf("[DEBUG] %s\n", s)
	default:
		logger.Warnf("Unhandled MSP frame %d with payload %v\n", fr.Code, fr.Payload)
	}
	return nil
}

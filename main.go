package main

import (
	"github.com/gtu-nova/nova-cli/msp"
	"github.com/tarm/serial"
	"strings"

	"flag"
	"github.com/gtu-nova/nova-cli/fc"
	"os"
	"github.com/kellydunn/golang-geo"

	"github.com/sirupsen/logrus"
)

var (
	portName = flag.String("p", "", "Serial port")
)

var logger = logrus.New()

func initLogger() {
	logger.SetOutput(os.Stdout)
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



	fr, _ = fcv.WriteCmd(msp.RawGps)
	if fr != nil {
		var r msp.RawGpsData
		_ = fr.Read(&r)

		lat := float64(r.Latitude) / 10_000_000
		long := float64(r.Longitude) / 10_000_000

		logger.Infof("Fix: %d, Sat: %d, Lat: %f, Long: %f, Alt: %d\n", r.FixType, r.NumSat,
			lat, long, r.Altitude)

		point := geo.NewPoint(lat, long)
		var newPoint *geo.Point
		flag := uint8(0)

		for i := 0; i < 6; i++ {
			if i == 5 {
				flag = 0xa5
			}
			newPoint = point.PointAtDistanceAndBearing(0.005, float64(60*i))
			fr, _ = fcv.WriteCmd(msp.SetWp, msp.SetGetWpData{
				WpNo:     uint8(i + 1),
				Action:    	1,
				Latitude:  uint32(newPoint.Lat() * 10_000_000),
				Longitude: uint32(newPoint.Lng() * 10_000_000),
				Altitude:  1000,
				P1: 3000,
				Flag:      flag,
			})
		}

		fr, _ = fcv.WriteCmd(msp.WpMissionSave, uint8(0))

		fr, err = fcv.WriteCmd(msp.Wp, uint8(1))
		if fr != nil {
			var rx msp.SetGetWpData
			_ = fr.Read(&rx)
			logger.Infof("No: %d, Lat: %f, Lon: %f, Alt: %d\n", rx.WpNo, float64(rx.Latitude)/10_000_000, float64(rx.Longitude)/10_000_000, rx.Altitude)
		}
	}




	//func() {
	//	go func() {
	//		for {
	//			select {
	//			case _ = <-ticker.C:
	//				_, _ = fcv.WriteCmd(msp.RawGps)
	//			}
	//		}
	//	}()
	//}()

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

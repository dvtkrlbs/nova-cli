package main

import (
	"github.com/gtu-nova/nova-cli/msp"
	"github.com/tarm/serial"
	"strings"
	"time"

	"flag"
	"github.com/gtu-nova/nova-cli/fc"
	"os"
	//"github.com/kellydunn/golang-geo"

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

	fcv, err := fc.NewFC(port, handleFrame, logger)
	if err != nil {
		logger.Fatal(err)
	}

	defer fcv.Close()

	//ticker := time.NewTicker(200 * time.Millisecond)

	_, _ = fcv.WriteCmd(msp.ApiVersion)
	_, _ = fcv.WriteCmd(msp.FcVariant)
	_, _ = fcv.WriteCmd(msp.FcVersion)
	_, _ = fcv.WriteCmd(msp.BoardInfo)
	_, _ = fcv.WriteCmd(msp.BuildInfo)



	go fcv.WriteCmd(msp.SetWp, msp.SetGetWpData{
		WpNo:      1,
		Action:    1,
		Latitude:  410194400,
		Longitude: 290771002,
		Altitude:  5000,
		Flag:      0xa5,
	})
	time.Sleep(400 * time.Millisecond)
	_, _ = fcv.WriteCmd(msp.WpMissionSave, uint8(0))
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

	time.Sleep(5 * time.Second)
}

func handleFrame(fr msp.Frame, fc *fc.FC) error {
	switch fr.Code {
	case msp.ApiVersion:
		logger.Infof("MSP API version %d.%d (protocol %d)\n", fr.Payload[1], fr.Payload[2], fr.Payload[0])
	case msp.FcVariant:
		_fc.variant = string(fr.Payload)
		_fc.printInfo()
	case msp.FcVersion:
		_fc.versionMajor = fr.Payload[0]
		_fc.versionMinor = fr.Payload[1]
		_fc.versionPatch = fr.Payload[2]
		_fc.printInfo()
	case msp.BoardInfo:
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
	case msp.BuildInfo:
		buildDate := string(fr.Payload[:11])
		buildTime := string(fr.Payload[11:19])
		// XXX: Revision is 8 characters in iNav but 7 in BF/CF
		rev := string(fr.Payload[19:])
		logger.Infof("Build %s (built on %s @ %s)\n", rev, buildDate, buildTime)
	case msp.Reboot:
		logger.Warn("Rebooting board...\n")

	case msp.RawGps:
		var r msp.RawGpsData
		_ = fr.Read(&r)

		logger.Infof("Fix: %d, Sat: %d, Lat: %f, Long: %f, Alt: %d\n", r.FixType, r.NumSat,
			float64(r.Latitude)/10_000_000, float64(r.Longitude)/10_000_000, r.Altitude)
	case msp.Wp:
		var r msp.SetGetWpData
		_ = fr.Read(&r)
		logger.Infof("No: %d, Lat: %f, Lon: %f, Alt: %d\n", r.WpNo, float64(r.Latitude)/10_000_000, float64(r.Longitude)/10_000_000, r.Altitude)
	case msp.DebugMsg:
		s := strings.Trim(string(fr.Payload), " \r\n\t\x00")
		logger.Debugf("[DEBUG] %s\n", s)
	case msp.SetWp:
	// Nothing to do for these
	default:
		logger.Warnf("Unhandled MSP frame %d with payload %v\n", fr.Code, fr.Payload)
	}
	return nil
}

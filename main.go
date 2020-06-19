package main

import (
	"github.com/gtu-nova/msp"
	"github.com/tarm/serial"
	"runtime"
	"strings"
	"time"

	"flag"
	"github.com/gtu-nova/nova-cli/fc"
	"os"
	//"gopkg.in/Billups/golang-geo.v2"

	"github.com/sirupsen/logrus"
)

var (
	portName = flag.String("p", "", "Serial port")
)

//for {
//// Trying to connect on macOS when the port dev file is
//// not present would cause an USB hub reset.
//if f.portIsPresent() {
//if err == nil {
//f.logger.Info("Reconnected to Flight Controller\n")
//f.msp = m
//f.updateInfo()
//return nil
//}
//}
//time.Sleep(time.Millisecond)
//}

//// We want to avoid an EOF from the uart at all costs,
//// so close the current port and open another one to ensure
//// the goroutine reading from the port stops even if the
//// board reboots very fast.
//m := f.msp
//f.msp = nil
//_ = m.Close()
//time.Sleep(time.Second)
//mm, err := msp.New(f.opts.PortName, f.opts.BaudRate, f.logger)
//if err != nil {
//return err
//}
//_, _ = m.WriteCmd(msp.Reboot)
//_ = mm.Close()

//func (f *FC) portIsPresent() bool {
//	if _, err := os.Stat(f.opts.PortName); os.IsNotExist(err) {
//		return false
//	}
//
//	return true
//}

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

type SCP struct {
	name string
	p    *serial.Port
}

func (scp *SCP) Reconnect() error {
	for {
		// Trying to connect on macOS when the scp dev file is
		// not present would cause an USB hub reset.
		if scp.portIsPresent() {
			port, err := serial.OpenPort(&serial.Config{
				Name: *portName,
				Baud: 115200,
			})
			if err != nil {
				return err
			}
			scp.p = port
			return nil
		}
		time.Sleep(time.Millisecond)
	}

}

func (scp *SCP) portIsPresent() bool {
	if runtime.GOOS == "windows" {
		return true
	}
	_, err := os.Stat(scp.name)
	return err == nil
}

func (scp *SCP) Read(p []byte) (n int, err error) {
	return scp.p.Read(p)
}

func (scp *SCP) Write(p []byte) (n int, err error) {
	return scp.p.Write(p)
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

func (f *myFC) reset() {
	f.variant = ""
	f.versionMajor = 0
	f.versionMinor = 0
	f.versionPatch = 0
	f.boardID = ""
	f.targetName = ""
	f.Features = 0
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

	fcv, err := fc.NewFC(&SCP{p: port, name: *portName}, handleFrame, logger)
	if err != nil {
		logger.Fatal(err)
	}

	defer fcv.Close()

	ticker := time.NewTicker(200 * time.Millisecond)

	_, _ = fcv.WriteCmd(msp.ApiVersion)
	_, _ = fcv.WriteCmd(msp.FcVariant)
	_, _ = fcv.WriteCmd(msp.FcVersion)
	_, _ = fcv.WriteCmd(msp.BoardInfo)
	_, _ = fcv.WriteCmd(msp.BuildInfo)

	func() {
		go func() {
			for {
				select {
				case _ = <-ticker.C:
					_, _ = fcv.WriteCmd(msp.RawGps)
				}
			}
		}()
	}()

	//time.AfterFunc(3 * time.Second, func() {
	//	_, _ = fcv.WriteCmd(msp.Reboot)
	//})

	time.Sleep(15 * time.Second)
}

func handleFrame(fr msp.Frame, f *fc.FC) error {
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
